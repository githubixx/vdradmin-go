package http

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// HLSProxy manages HLS transcoding streams for Watch TV.
// It spawns ffmpeg processes per channel and serves HLS playlists/segments.
type HLSProxy struct {
	logger          *slog.Logger
	backendTemplate string   // e.g. "http://127.0.0.1:3000/{channel}"
	workDir         string   // temp directory for HLS files
	streams         sync.Map // map[string]*hlsStream
	mu              sync.Mutex
}

type hlsStream struct {
	channelNum string
	cmd        *exec.Cmd
	ctx        context.Context
	cancel     context.CancelFunc
	hlsDir     string
	lastAccess time.Time
	ready      chan struct{} // signals when first segment is ready
	mu         sync.Mutex
}

// NewHLSProxy creates a new HLS proxy instance.
func NewHLSProxy(logger *slog.Logger, backendTemplate string) (*HLSProxy, error) {
	if logger == nil {
		logger = slog.Default()
	}

	workDir := filepath.Join(os.TempDir(), "vdradmin-hls")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create HLS work directory: %w", err)
	}

	p := &HLSProxy{
		logger:          logger,
		backendTemplate: backendTemplate,
		workDir:         workDir,
	}

	// Start cleanup goroutine to stop idle streams
	go p.cleanupLoop()

	return p, nil
}

// GetPlaylist serves the HLS playlist for a channel.
func (p *HLSProxy) GetPlaylist(w http.ResponseWriter, r *http.Request, channelNum string) {
	stream, err := p.ensureStream(channelNum)
	if err != nil {
		http.Error(w, fmt.Sprintf("Stream error: %v", err), http.StatusInternalServerError)
		return
	}

	stream.touch()

	playlistPath := filepath.Join(stream.hlsDir, "index.m3u8")

	// Block long enough for typical DVB tuning + ffmpeg startup.
	// Chromium may not recover from an initial <video src> error, so prefer returning
	// a 200 once the playlist exists.
	const maxWait = 12 * time.Second
	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if st, err := os.Stat(playlistPath); err == nil && st.Size() > 0 {
			break
		}

		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Playlist not ready", http.StatusServiceUnavailable)
			return
		case <-stream.ready:
			// ready is a hint; loop will check file existence/size.
		case <-ticker.C:
		}
	}

	w.Header().Set("Content-Type", "application/x-mpegURL")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Accept-Ranges", "none")

	f, err := os.Open(playlistPath)
	if err != nil {
		http.Error(w, "Playlist not available", http.StatusServiceUnavailable)
		return
	}
	defer f.Close()

	// Always serve full content with 200 to keep HLS clients happy.
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

// StopAll stops all active HLS streams immediately.
// This is used on channel switch to free DVB tuners deterministically.
func (p *HLSProxy) StopAll() {
	p.streams.Range(func(key, value any) bool {
		stream := value.(*hlsStream)
		stream.stop()
		p.streams.Delete(key)
		return true
	})
}

// GetSegment serves an HLS segment for a channel.
func (p *HLSProxy) GetSegment(w http.ResponseWriter, r *http.Request, channelNum, segmentName string) {
	stream, err := p.getStream(channelNum)
	if err != nil {
		http.Error(w, "Stream not found", http.StatusNotFound)
		return
	}

	stream.touch()

	segmentPath := filepath.Join(stream.hlsDir, segmentName)
	if _, err := os.Stat(segmentPath); err != nil {
		http.Error(w, "Segment not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "video/MP2T")
	w.Header().Set("Cache-Control", "max-age=10")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Accept-Ranges", "none")

	f, err := os.Open(segmentPath)
	if err != nil {
		http.Error(w, "Segment not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

// ensureStream gets or creates an HLS stream for the given channel.
func (p *HLSProxy) ensureStream(channelNum string) (*hlsStream, error) {
	// Check if stream already exists
	if val, ok := p.streams.Load(channelNum); ok {
		return val.(*hlsStream), nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring lock
	if val, ok := p.streams.Load(channelNum); ok {
		return val.(*hlsStream), nil
	}

	// Create new stream
	backendURL := strings.ReplaceAll(p.backendTemplate, "{channel}", channelNum)
	hlsDir := filepath.Join(p.workDir, channelNum)

	// Clean up any existing directory from previous stream
	os.RemoveAll(hlsDir)

	if err := os.MkdirAll(hlsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create HLS directory: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Build ffmpeg command for HLS transcoding
	// Input options:
	// -fflags: genpts (generate timestamps), discardcorrupt (skip bad packets)
	// -probesize/-analyzeduration: increased to capture keyframe with SPS/PPS
	// -i: input URL from streamdev
	// Stream mapping:
	// -map 0:v:0: first video stream only
	// -map 0:a:0?: first audio stream if present
	// Video encoding:
	// -c:v libx264: H.264 re-encoding (creates clean stream from corrupt input)
	// -preset ultrafast -tune zerolatency: optimize for live streaming
	// -g 50 -keyint_min 50: force regular keyframes every 50 frames (~2 sec)
	// -x264-params: repeat-headers=1 ensures SPS/PPS in every keyframe
	// Audio encoding:
	// -c:a aac: AAC audio (browser compatible)
	// HLS output:
	// -f hls: HLS muxer
	// -hls_time 2: 2-second segments
	// -hls_list_size 10: keep last 10 segments in playlist
	// -hls_flags: delete old segments, omit endlist for live stream
	// -start_number 0: start from segment 0
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-loglevel", "error",
		"-fflags", "+genpts+discardcorrupt",
		"-probesize", "5000000",
		"-analyzeduration", "2000000",
		"-i", backendURL,
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-g", "50",
		"-keyint_min", "50",
		"-x264-params", "repeat-headers=1",
		"-sc_threshold", "0",
		"-c:a", "aac",
		"-b:a", "128k",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "5",
		"-hls_flags", "omit_endlist",
		"-hls_allow_cache", "0",
		"-start_number", "0",
		"-hls_segment_filename", filepath.Join(hlsDir, "segment-%d.ts"),
		filepath.Join(hlsDir, "index.m3u8"),
	)

	// Ensure we can kill the entire ffmpeg process group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Capture stderr for error logging
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	stream := &hlsStream{
		channelNum: channelNum,
		cmd:        cmd,
		ctx:        ctx,
		cancel:     cancel,
		hlsDir:     hlsDir,
		lastAccess: time.Now(),
		ready:      make(chan struct{}),
	}

	p.streams.Store(channelNum, stream)

	// Monitor for first segment in background
	go func() {
		playlistPath := filepath.Join(hlsDir, "index.m3u8")
		segment0Path := filepath.Join(hlsDir, "segment-0.ts")

		// Wait up to 20 seconds for playlist and first segment
		// VDR needs time to tune the DVB card (can take 10+ seconds)
		for i := 0; i < 200; i++ {
			// Check if playlist exists and has content
			if stat, err := os.Stat(playlistPath); err == nil && stat.Size() > 100 {
				// Check if first segment exists and has some data
				if segStat, segErr := os.Stat(segment0Path); segErr == nil && segStat.Size() > 100000 {
					// Wait a bit more for the segment to finish writing
					time.Sleep(500 * time.Millisecond)
					// Stream is ready
					close(stream.ready)
					p.logger.Info("stream ready", slog.String("channel", channelNum), slog.Duration("startup_time", time.Since(stream.lastAccess)))
					return
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		p.logger.Warn("stream startup timeout", slog.String("channel", channelNum))
		close(stream.ready) // Signal anyway to prevent hanging
	}()

	// Monitor ffmpeg process
	go func() {
		// Read stderr for error messages
		var stderrBuf strings.Builder
		if stderrPipe != nil {
			io.Copy(&stderrBuf, stderrPipe)
		}

		if err := cmd.Wait(); err != nil {
			errMsg := strings.TrimSpace(stderrBuf.String())
			if errMsg != "" {
				p.logger.Error("ffmpeg error", slog.String("channel", channelNum), slog.String("stderr", errMsg), slog.Any("exit_error", err))
			} else {
				p.logger.Warn("ffmpeg exited", slog.String("channel", channelNum), slog.Any("error", err))
			}
		}
		p.streams.Delete(channelNum)
		os.RemoveAll(hlsDir)
	}()

	p.logger.Info("started HLS stream", slog.String("channel", channelNum), slog.String("backend", backendURL))

	return stream, nil
}

// getStream retrieves an existing stream.
func (p *HLSProxy) getStream(channelNum string) (*hlsStream, error) {
	val, ok := p.streams.Load(channelNum)
	if !ok {
		return nil, fmt.Errorf("stream not found for channel %s", channelNum)
	}
	return val.(*hlsStream), nil
}

// cleanupLoop periodically stops idle streams.
// With aggressive channel-switch cleanup, this mainly handles abandoned sessions
func (p *HLSProxy) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		p.streams.Range(func(key, value any) bool {
			stream := value.(*hlsStream)
			stream.mu.Lock()
			idle := now.Sub(stream.lastAccess)
			stream.mu.Unlock()

			// Stop streams idle for more than 5 minutes (very conservative)
			// Most cleanup happens on channel switch
			if idle > 5*time.Minute {
				p.logger.Info("stopping abandoned HLS stream", slog.String("channel", stream.channelNum), slog.Duration("idle", idle))
				stream.stop()
				p.streams.Delete(key)
			}
			return true
		})
	}
}

// Shutdown stops all active streams.
func (p *HLSProxy) Shutdown() {
	p.streams.Range(func(key, value any) bool {
		stream := value.(*hlsStream)
		stream.stop()
		return true
	})
}

func (s *hlsStream) touch() {
	s.mu.Lock()
	s.lastAccess = time.Now()
	s.mu.Unlock()
}

func (s *hlsStream) stop() {
	s.cancel()
	if s.cmd != nil && s.cmd.Process != nil {
		// Prefer killing the whole process group to avoid leaving children around.
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		_ = s.cmd.Process.Kill()
	}
	os.RemoveAll(s.hlsDir)
}

// IsHLSProxyEnabled checks if HLS proxy should be enabled based on config.
func IsHLSProxyEnabled(backendURL string) bool {
	return strings.TrimSpace(backendURL) != ""
}

// BuildHLSProxyURL builds the internal HLS playlist URL for a given channel number.
func BuildHLSProxyURL(channelNum int) string {
	return fmt.Sprintf("/watch/stream/%d/index.m3u8", channelNum)
}

// ParseChannelFromPath extracts channel number from HLS proxy path.
// Path format: /watch/stream/{channel}/index.m3u8 or /watch/stream/{channel}/segment-N.ts
func ParseChannelFromPath(path string) (string, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/watch/stream/"), "/")
	if len(parts) < 1 {
		return "", fmt.Errorf("invalid HLS path")
	}
	channelNum := parts[0]
	if _, err := strconv.Atoi(channelNum); err != nil {
		return "", fmt.Errorf("invalid channel number: %s", channelNum)
	}
	return channelNum, nil
}
