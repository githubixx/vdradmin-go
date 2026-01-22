package svdrp

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

// GrabJpeg captures a snapshot as JPEG via SVDRP GRAB.
// This mirrors the classic vdradmin-am behavior (GRAB .jpg 80 <w> <h>) and expects
// base64-encoded payload in the SVDRP response.
func (c *Client) GrabJpeg(ctx context.Context, width int, height int) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid dimensions: %dx%d", width, height)
	}

	return withRetry(ctx, c, func() ([]byte, error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		cmd := fmt.Sprintf("GRAB .jpg 80 %d %d", width, height)
		if err := c.sendCommandLocked(cmd); err != nil {
			return nil, err
		}

		lines, err := c.readResponseLocked()
		if err != nil {
			return nil, err
		}

		var b strings.Builder
		b.Grow(64 * 1024)
		for _, ln := range lines {
			if strings.Contains(ln, "Grab image failed") {
				return nil, fmt.Errorf("grab image failed")
			}
			if strings.Contains(ln, "Grabbed image") {
				continue
			}
			b.WriteString(strings.TrimSpace(ln))
		}

		payload := b.String()
		payload = strings.Join(strings.Fields(payload), "")
		if payload == "" {
			return nil, fmt.Errorf("empty grab payload")
		}

		img, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("decode grab payload: %w", err)
		}
		return img, nil
	})
}

var recordingPathTimestampRe = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\.(\d{2})\.(\d{2})\.(\d{2})\b`)
var recordingListLeadingDateTimeRe = regexp.MustCompile(`^\s*(\d{4}-\d{2}-\d{2})\s+(\d{2}):(\d{2})\b`)
var recordingShortDateRe = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{2}$`)
var recordingClockRe = regexp.MustCompile(`^\d{2}:\d{2}$`)

// Recording length tokens may include flags like '*' and '!' (e.g. "4:22*!" or "0:22*").
var recordingLengthRe = regexp.MustCompile(`^\d+:\d{2}[*!]{0,2}$`)

// Returned when we failed while writing/flushing the command to the socket.
// Retrying after this is usually safe because the command likely didn't reach VDR.
var errSVDRPSendFailed = errors.New("svdrp send failed")

// Client implements the SVDRP protocol for VDR communication.
type Client struct {
	host    string
	port    int
	timeout time.Duration

	mu   sync.Mutex
	conn net.Conn
	rw   *bufio.ReadWriter
}

// UpdateConnection updates the target host/port/timeout and forces a reconnect.
// It is safe to call concurrently.
func (c *Client) UpdateConnection(host string, port int, timeout time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if host != "" {
		c.host = host
	}
	if port > 0 {
		c.port = port
	}
	if timeout > 0 {
		c.timeout = timeout
	}

	// Force reconnect with updated parameters.
	c.closeConnectionLocked()
}

// NewClient creates a new SVDRP client.
func NewClient(host string, port int, timeout time.Duration) *Client {
	return &Client{host: host, port: port, timeout: timeout}
}

// Connect establishes a connection to VDR.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil
	}

	dialer := &net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", c.host, c.port))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	// Read welcome message.
	if _, err := c.readResponseLocked(); err != nil {
		c.closeConnectionLocked()
		return fmt.Errorf("failed to read welcome: %w", err)
	}

	return nil
}

// Close closes the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	// Best-effort QUIT (ignore errors on broken connections).
	_, _ = c.rw.WriteString("QUIT\r\n")
	_ = c.rw.Flush()

	err := c.conn.Close()
	c.conn = nil
	c.rw = nil
	return err
}

// Ping checks if VDR is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := withRetry(ctx, c, func() (struct{}, error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if err := c.sendCommandLocked("STAT disk"); err != nil {
			return struct{}{}, err
		}
		_, err := c.readResponseLocked()
		return struct{}{}, err
	})
	return err
}

// GetChannels retrieves all channels.
func (c *Client) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	return withRetry(ctx, c, func() ([]domain.Channel, error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if err := c.sendCommandLocked("LSTC"); err != nil {
			return nil, err
		}

		lines, err := c.readResponseLocked()
		if err != nil {
			return nil, err
		}

		channels := make([]domain.Channel, 0, len(lines))
		for i, line := range lines {
			trim := strings.TrimSpace(line)
			parts := strings.Fields(trim)
			if len(parts) == 2 && (parts[1] == "channels" || parts[1] == "channel") {
				continue
			}

			ch := parseChannel(i+1, line)
			if ch.ID == "" && ch.Name == "" {
				continue
			}
			channels = append(channels, ch)
		}
		return channels, nil
	})
}

// GetEPG retrieves EPG data.
func (c *Client) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	return withRetry(ctx, c, func() ([]domain.EPGEvent, error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		cmd := "LSTE"
		if channelID != "" {
			if !at.IsZero() {
				cmd = fmt.Sprintf("LSTE %s %d", channelID, at.Unix())
			} else {
				cmd = fmt.Sprintf("LSTE %s", channelID)
			}
		}

		if err := c.sendCommandLocked(cmd); err != nil {
			return nil, err
		}

		lines, err := c.readResponseLocked()
		if err != nil {
			// Some channels simply have no EPG (e.g. no schedule). Treat as empty.
			if channelID != "" && isSVDRPNoSchedule(err) {
				return []domain.EPGEvent{}, nil
			}

			// Some VDR versions don't support the optional timestamp argument.
			if channelID != "" && !at.IsZero() && isSVDRPUnknownOption(err) {
				cmd = fmt.Sprintf("LSTE %s", channelID)
				if err := c.sendCommandLocked(cmd); err != nil {
					return nil, err
				}
				lines, err = c.readResponseLocked()
				if err != nil {
					if isSVDRPNoSchedule(err) {
						return []domain.EPGEvent{}, nil
					}
					return nil, err
				}
				return parseEPGEvents(lines), nil
			}

			return nil, err
		}

		return parseEPGEvents(lines), nil
	})
}

func isSVDRPUnknownOption(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "svdrp error 501") || strings.Contains(msg, "unknown option")
}

func isSVDRPNoSchedule(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "svdrp error 550") || strings.Contains(msg, "no schedule")
}

func isSVDRPRecordingNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// VDR uses 550 for various "not found" conditions; when querying a recording path
	// this usually indicates a stale in-memory entry for a recording deleted on disk.
	if !strings.Contains(msg, "svdrp error 550") {
		return false
	}
	return strings.Contains(msg, "record") || strings.Contains(msg, "not found") || strings.Contains(msg, "unknown")
}

// GetTimers retrieves all timers.
func (c *Client) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	return withRetry(ctx, c, func() ([]domain.Timer, error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if err := c.sendCommandLocked("LSTT"); err != nil {
			return nil, err
		}

		lines, err := c.readResponseLocked()
		if err != nil {
			return nil, err
		}

		timers := make([]domain.Timer, 0, len(lines))
		for _, line := range lines {
			t, err := parseTimer(line)
			if err == nil {
				timers = append(timers, t)
			}
		}

		return timers, nil
	})
}

// CreateTimer creates a new timer.
func (c *Client) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	return withRetryWrite(ctx, c, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		timerStr := formatTimer(timer)
		if err := c.sendCommandLocked(fmt.Sprintf("NEWT %s", timerStr)); err != nil {
			return err
		}
		_, err := c.readResponseLocked()
		return err
	})
}

// UpdateTimer updates an existing timer.
func (c *Client) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	return withRetryWrite(ctx, c, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		timerStr := formatTimer(timer)
		if err := c.sendCommandLocked(fmt.Sprintf("MODT %d %s", timer.ID, timerStr)); err != nil {
			return err
		}
		_, err := c.readResponseLocked()
		return err
	})
}

// DeleteTimer deletes a timer.
func (c *Client) DeleteTimer(ctx context.Context, timerID int) error {
	return withRetryWrite(ctx, c, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		if err := c.sendCommandLocked(fmt.Sprintf("DELT %d", timerID)); err != nil {
			return err
		}
		_, err := c.readResponseLocked()
		return err
	})
}

// GetRecordings retrieves all recordings.
func (c *Client) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	return withRetry(ctx, c, func() ([]domain.Recording, error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		// If recordings were removed directly on disk (outside VDR), VDR may still
		// have a stale in-memory list. Best-effort trigger a rescan/update.
		// Not all VDR versions expose this via SVDRP, so ignore protocol errors.
		if err := c.sendCommandLocked("UPDR"); err == nil {
			if _, err := c.readResponseLocked(); err != nil {
				if isTransientConnErr(err) {
					return nil, err
				}
				// Ignore SVDRP protocol errors (e.g. unknown/unsupported command).
			}
		} else if isTransientConnErr(err) {
			return nil, err
		}

		if err := c.sendCommandLocked("LSTR"); err != nil {
			return nil, err
		}

		lines, err := c.readResponseLocked()
		if err != nil {
			return nil, err
		}

		recordings := make([]domain.Recording, 0, len(lines))
		for _, line := range lines {
			r, err := parseRecording(line)
			if err == nil {
				// Resolve the actual recording directory from VDR and filter out
				// entries that no longer exist on disk (e.g. deleted out-of-band).
				dirPath, err := c.getRecordingDirPathLocked(r.Path)
				if err != nil {
					if isTransientConnErr(err) {
						return nil, err
					}
					// If VDR reports the recording does not exist, drop the entry.
					if isSVDRPRecordingNotFound(err) {
						continue
					}
					// Otherwise: protocol/feature mismatch; keep the entry.
					recordings = append(recordings, r)
					continue
				}
				if dirPath != "" {
					r.DiskPath = dirPath
					if _, statErr := os.Stat(dirPath); statErr != nil {
						if os.IsNotExist(statErr) {
							continue
						}
						// Permission/mount issues: don't hide recordings we can't verify.
					}

					// Some VDR setups don't include complete metadata in LSTR.
					// Best-effort enrich from the recording's info file.
					if info, infoErr := readRecordingInfoMeta(dirPath); infoErr == nil {
						if strings.TrimSpace(r.Subtitle) == "" && strings.TrimSpace(info.Subtitle) != "" {
							r.Subtitle = info.Subtitle
						}
						if strings.TrimSpace(r.Description) == "" && strings.TrimSpace(info.Description) != "" {
							r.Description = info.Description
						}
						if strings.TrimSpace(r.Channel) == "" && strings.TrimSpace(info.Channel) != "" {
							r.Channel = info.Channel
						}
						if r.Date.IsZero() && !info.Start.IsZero() {
							r.Date = info.Start
						}
						if r.Date.IsZero() {
							// Some entries (e.g. radio) have info E=0 and may not have a parsable ID/path.
							r.Date = parseRecordingDateFromPath(dirPath)
						}
						if r.Length <= 0 && info.Duration > 0 {
							r.Length = info.Duration
						}
						// If LSTR title looks like it accidentally includes time/length, prefer info title.
						if strings.TrimSpace(info.Title) != "" && (strings.TrimSpace(r.Title) == "" || looksLikeTimeLengthPrefix(r.Title)) {
							r.Title = info.Title
						}
						// Fallback: if still no title, infer from the folder structure.
						if strings.TrimSpace(r.Title) == "" {
							if inferred := inferRecordingTitleFromDir(dirPath); inferred != "" {
								r.Title = inferred
							}
						}
					}
				}
				recordings = append(recordings, r)
			}
		}

		return recordings, nil
	})
}

type recordingInfoMeta struct {
	Title       string
	Subtitle    string
	Description string
	Channel     string
	Start       time.Time
	Duration    time.Duration
}

func looksLikeTimeLengthPrefix(title string) bool {
	f := strings.Fields(strings.TrimSpace(title))
	if len(f) < 2 {
		return false
	}
	return recordingClockRe.MatchString(f[0]) && recordingLengthRe.MatchString(f[1])
}

func inferRecordingTitleFromDir(recordingDir string) string {
	// Typical VDR folder layout:
	//   /video/<Title>/_/2025-07-05.23.00.77-0.rec
	// or
	//   /video/<Folder>/<Title>/_/2025-...rec
	// where "_" is used as a placeholder segment.
	//
	// We pick the nearest non-"_" parent folder name and normalize underscores.
	if recordingDir == "" {
		return ""
	}
	parent := filepath.Base(filepath.Dir(recordingDir))
	if parent == "" || parent == "." || parent == string(filepath.Separator) {
		return ""
	}
	if parent == "_" {
		parent = filepath.Base(filepath.Dir(filepath.Dir(recordingDir)))
	}
	parent = strings.TrimSpace(parent)
	if parent == "" || parent == "_" {
		return ""
	}
	parent = strings.ReplaceAll(parent, "_", " ")
	parent = strings.TrimSpace(parent)
	parent = strings.Join(strings.Fields(parent), " ")
	return parent
}

func readRecordingInfoSubtitle(recordingDir string) (string, error) {
	meta, err := readRecordingInfoMeta(recordingDir)
	if err != nil {
		return "", err
	}
	return meta.Subtitle, nil
}

func readRecordingInfoMeta(recordingDir string) (recordingInfoMeta, error) {
	infoPath := filepath.Join(recordingDir, "info")
	b, err := os.ReadFile(infoPath)
	if err != nil {
		return recordingInfoMeta{}, err
	}

	var out recordingInfoMeta
	scanner := bufio.NewScanner(strings.NewReader(string(b)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.HasPrefix(line, "T ") {
			if out.Title == "" {
				out.Title = strings.TrimSpace(strings.TrimPrefix(line, "T "))
			}
			continue
		}
		if strings.HasPrefix(line, "S ") {
			if out.Subtitle == "" {
				out.Subtitle = strings.TrimSpace(strings.TrimPrefix(line, "S "))
			}
			continue
		}
		if strings.HasPrefix(line, "D ") {
			if out.Description == "" {
				out.Description = strings.TrimSpace(strings.TrimPrefix(line, "D "))
			}
			continue
		}
		if strings.HasPrefix(line, "C ") {
			// Example: "C S19.2E-1-1089-12003 RTL Television"
			cLine := strings.TrimSpace(strings.TrimPrefix(line, "C "))
			parts := strings.Fields(cLine)
			if len(parts) >= 2 {
				out.Channel = strings.TrimSpace(strings.Join(parts[1:], " "))
			} else {
				out.Channel = strings.TrimSpace(cLine)
			}
			continue
		}
		if strings.HasPrefix(line, "E ") {
			// Example: "E <eventid> <startUnix> <durationSec> ..."
			parts := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "E ")))
			if len(parts) >= 3 {
				startUnix, err1 := strconv.ParseInt(parts[1], 10, 64)
				durSec, err2 := strconv.ParseInt(parts[2], 10, 64)
				if err1 == nil && err2 == nil {
					out.Start = time.Unix(startUnix, 0).In(time.Local)
					if durSec > 0 {
						out.Duration = time.Duration(durSec) * time.Second
					}
				}
			}
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return recordingInfoMeta{}, err
	}
	return out, nil
}

func (c *Client) getRecordingDirPathLocked(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", nil
	}

	if err := c.sendCommandLocked(fmt.Sprintf("LSTR %s path", id)); err != nil {
		return "", err
	}
	lines, err := c.readResponseLocked()
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.TrimSpace(lines[0]), nil
}

// GetRecordingDir resolves the on-disk directory path for a recording.
func (c *Client) GetRecordingDir(ctx context.Context, recordingID string) (string, error) {
	return withRetry(ctx, c, func() (string, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.getRecordingDirPathLocked(recordingID)
	})
}

// DeleteRecording deletes a recording.
func (c *Client) DeleteRecording(ctx context.Context, path string) error {
	return withRetryWrite(ctx, c, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		if err := c.sendCommandLocked(fmt.Sprintf("DELR %s", path)); err != nil {
			return err
		}
		_, err := c.readResponseLocked()
		return err
	})
}

// GetCurrentChannel returns the current channel.
func (c *Client) GetCurrentChannel(ctx context.Context) (string, error) {
	return withRetry(ctx, c, func() (string, error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if err := c.sendCommandLocked("CHAN"); err != nil {
			return "", err
		}
		lines, err := c.readResponseLocked()
		if err != nil || len(lines) == 0 {
			return "", err
		}
		return strings.TrimSpace(lines[0]), nil
	})
}

// SetCurrentChannel switches to a channel.
func (c *Client) SetCurrentChannel(ctx context.Context, channelID string) error {
	return withRetryWrite(ctx, c, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		if err := c.sendCommandLocked(fmt.Sprintf("CHAN %s", channelID)); err != nil {
			return err
		}
		_, err := c.readResponseLocked()
		return err
	})
}

// SendKey sends a remote control key.
func (c *Client) SendKey(ctx context.Context, key string) error {
	return withRetryWrite(ctx, c, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		if err := c.sendCommandLocked(fmt.Sprintf("HITK %s", key)); err != nil {
			return err
		}
		_, err := c.readResponseLocked()
		return err
	})
}

func (c *Client) ensureConnected(ctx context.Context) error {
	c.mu.Lock()
	connected := c.conn != nil
	c.mu.Unlock()
	if !connected {
		return c.Connect(ctx)
	}
	return nil
}

// withRetry executes a function with exponential backoff and reconnection on transient connection errors.
// It is intentionally conservative and only retries errors that look like broken sockets (e.g. broken pipe, EOF).
func withRetry[T any](ctx context.Context, c *Client, fn func() (T, error)) (T, error) {
	var zero T

	const maxAttempts = 3
	backoff := 60 * time.Millisecond
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.ensureConnected(ctx); err != nil {
			return zero, err
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err

		if !isTransientConnErr(err) || attempt == maxAttempts {
			return zero, err
		}

		// Force reconnection before next try.
		c.mu.Lock()
		c.closeConnectionLocked()
		c.mu.Unlock()

		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return zero, ctx.Err()
		case <-t.C:
		}

		backoff *= 2
		if backoff > 600*time.Millisecond {
			backoff = 600 * time.Millisecond
		}
	}

	return zero, lastErr
}

// withRetryWrite retries only when sending the SVDRP command failed.
// This avoids retrying non-idempotent operations after an unknown server-side execution.
func withRetryWrite(ctx context.Context, c *Client, fn func() error) error {
	const maxAttempts = 3
	backoff := 60 * time.Millisecond

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.ensureConnected(ctx); err != nil {
			return err
		}

		err := fn()
		if err == nil {
			return nil
		}

		// Only retry if the send itself failed (broken pipe during write/flush).
		if !isTransientConnErr(err) || !errors.Is(err, errSVDRPSendFailed) || attempt == maxAttempts {
			return err
		}

		c.mu.Lock()
		c.closeConnectionLocked()
		c.mu.Unlock()

		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}

		backoff *= 2
		if backoff > 600*time.Millisecond {
			backoff = 600 * time.Millisecond
		}
	}

	return domain.ErrConnection
}

func isTransientConnErr(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, domain.ErrConnection) || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() || netErr.Temporary() {
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "use of closed network connection")
}

func (c *Client) sendCommandLocked(cmd string) error {
	if c.conn == nil {
		return domain.ErrConnection
	}

	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout))

	if _, err := c.rw.WriteString(cmd + "\r\n"); err != nil {
		c.closeConnectionLocked()
		return fmt.Errorf("%w: %w", errSVDRPSendFailed, err)
	}
	if err := c.rw.Flush(); err != nil {
		c.closeConnectionLocked()
		return fmt.Errorf("%w: %w", errSVDRPSendFailed, err)
	}
	return nil
}

func (c *Client) readResponseLocked() ([]string, error) {
	if c.conn == nil {
		return nil, domain.ErrConnection
	}

	_ = c.conn.SetReadDeadline(time.Now().Add(c.timeout))

	var lines []string
	for {
		line, err := c.rw.ReadString('\n')
		if err != nil {
			c.closeConnectionLocked()
			return nil, err
		}

		line = strings.TrimSpace(line)
		if len(line) < 4 {
			continue
		}

		code, err := strconv.Atoi(line[0:3])
		if err != nil {
			continue
		}

		if code >= 400 {
			return nil, fmt.Errorf("SVDRP error %d: %s", code, strings.TrimSpace(line[4:]))
		}

		// Continuation line.
		if line[3] == '-' {
			if len(line) > 4 {
				lines = append(lines, line[4:])
			}
			continue
		}

		// Final line.
		if len(line) > 4 {
			lines = append(lines, line[4:])
		}
		break
	}

	return lines, nil
}

func (c *Client) closeConnectionLocked() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.rw = nil
	}
}

func parseChannel(number int, line string) domain.Channel {
	chID, chNumber, chName, provider := parseSVDRPChannelHeader(strings.TrimSpace(line), number)
	if chID == "" && chName == "" {
		return domain.Channel{}
	}
	if chNumber == 0 {
		chNumber = number
	}
	if chID == "" {
		chID = strconv.Itoa(chNumber)
	}
	return domain.Channel{ID: chID, Number: chNumber, Name: chName, Provider: provider}
}

func parseSVDRPChannelHeader(text string, numberFallback int) (channelID string, channelNumber int, channelName string, provider string) {
	if text == "" {
		return "", 0, "", ""
	}

	parts := strings.SplitN(text, " ", 3)
	first := strings.TrimSpace(parts[0])

	// Cases we try to support:
	// 1) "<num> <channels.conf line>"
	// 2) "<num> <channel-id> <channels.conf line>"
	// 3) "<channel-id> <channels.conf line>"

	if n, err := strconv.Atoi(first); err == nil {
		channelNumber = n
		if len(parts) >= 2 {
			rest := strings.TrimSpace(text[len(first):])
			rest = strings.TrimSpace(rest)
			r2 := strings.SplitN(rest, " ", 2)
			if len(r2) >= 1 && looksLikeVDRChannelID(r2[0]) {
				channelID = r2[0]
				if len(r2) == 2 {
					channelName, provider = parseChannelNameProvider(r2[1])
				}
			} else {
				channelID = deriveChannelIDFromChannelsConf(rest)
				channelName, provider = parseChannelNameProvider(rest)
				if channelID == "" {
					channelID = first
				}
			}
		}
		return channelID, channelNumber, channelName, provider
	}

	if looksLikeVDRChannelID(first) {
		channelID = first
		if len(parts) >= 2 {
			rest := strings.TrimSpace(text[len(first):])
			rest = strings.TrimSpace(rest)
			channelName, provider = parseChannelNameProvider(rest)
		}
		return channelID, numberFallback, channelName, provider
	}

	channelID = deriveChannelIDFromChannelsConf(text)
	channelName, provider = parseChannelNameProvider(text)
	return channelID, numberFallback, channelName, provider
}

func looksLikeVDRChannelID(s string) bool {
	return strings.Contains(s, "-") && (strings.HasPrefix(s, "S") || strings.HasPrefix(s, "C") || strings.HasPrefix(s, "T") || strings.HasPrefix(s, "A"))
}

func parseChannelNameProvider(channelsConfLine string) (name string, provider string) {
	nameProvider := strings.SplitN(channelsConfLine, ":", 2)[0]
	nameParts := strings.SplitN(nameProvider, ";", 2)
	name = strings.TrimSpace(nameParts[0])
	if len(nameParts) == 2 {
		provider = strings.TrimSpace(nameParts[1])
	}
	return name, provider
}

func deriveChannelIDFromChannelsConf(channelsConfLine string) string {
	fields := strings.Split(channelsConfLine, ":")
	if len(fields) < 12 {
		return ""
	}
	source := strings.TrimSpace(fields[3])
	sid := strings.TrimSpace(fields[9])
	nid := strings.TrimSpace(fields[10])
	tid := strings.TrimSpace(fields[11])
	if source == "" || sid == "" || nid == "" || tid == "" {
		return ""
	}
	return fmt.Sprintf("%s-%s-%s-%s", source, nid, tid, sid)
}

func parseEPGEvents(lines []string) []domain.EPGEvent {
	events := make([]domain.EPGEvent, 0)
	var currentEvent *domain.EPGEvent
	currentChannelID := ""
	currentChannelName := ""
	currentChannelNumber := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "C ") {
			chID, chNum, chName, _ := parseSVDRPChannelHeader(strings.TrimSpace(line[2:]), 0)
			currentChannelID = chID
			currentChannelName = chName
			currentChannelNumber = chNum
			continue
		}

		if strings.HasPrefix(line, "E ") {
			if currentEvent != nil {
				events = append(events, *currentEvent)
			}
			currentEvent = parseEPGEventLine(line)
			if currentEvent != nil {
				currentEvent.ChannelID = currentChannelID
				currentEvent.ChannelNumber = currentChannelNumber
				currentEvent.ChannelName = currentChannelName
			}
			continue
		}

		if currentEvent == nil {
			continue
		}
		if strings.HasPrefix(line, "T ") {
			currentEvent.Title = line[2:]
		} else if strings.HasPrefix(line, "S ") {
			currentEvent.Subtitle = line[2:]
		} else if strings.HasPrefix(line, "D ") {
			currentEvent.Description += line[2:] + "\n"
		}
	}

	if currentEvent != nil {
		events = append(events, *currentEvent)
	}

	return events
}

func parseEPGEventLine(line string) *domain.EPGEvent {
	parts := strings.Fields(line[2:])
	if len(parts) < 3 {
		return nil
	}

	eventID, _ := strconv.Atoi(parts[0])
	startUnix, _ := strconv.ParseInt(parts[1], 10, 64)
	duration, _ := strconv.Atoi(parts[2])

	start := time.Unix(startUnix, 0)
	dur := time.Duration(duration) * time.Second

	return &domain.EPGEvent{EventID: eventID, Start: start, Stop: start.Add(dur), Duration: dur}
}

func parseTimer(line string) (domain.Timer, error) {
	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 {
		return domain.Timer{}, fmt.Errorf("invalid timer format")
	}

	timerID, _ := strconv.Atoi(parts[0])
	fields := strings.SplitN(parts[1], ":", 9)
	if len(fields) < 8 {
		return domain.Timer{}, fmt.Errorf("insufficient timer fields")
	}

	active := fields[0] == "1"
	priority, _ := strconv.Atoi(fields[5])
	lifetime, _ := strconv.Atoi(fields[6])

	daySpec := strings.TrimSpace(fields[2])
	startClock := parseTimerClock(fields[3])
	stopClock := parseTimerClock(fields[4])

	day := parseTimerDay(daySpec)
	if day.IsZero() && isWeekdayMask(daySpec) {
		day = nextOccurrenceDay(daySpec, startClock, time.Now())
	}

	var startTime time.Time
	var stopTime time.Time
	if !day.IsZero() && startClock >= 0 {
		startTime = time.Date(day.Year(), day.Month(), day.Day(), startClock/60, startClock%60, 0, 0, day.Location())
	}
	if !day.IsZero() && stopClock >= 0 {
		stopTime = time.Date(day.Year(), day.Month(), day.Day(), stopClock/60, stopClock%60, 0, 0, day.Location())
		if !startTime.IsZero() && stopTime.Before(startTime) {
			stopTime = stopTime.Add(24 * time.Hour)
		}
	}

	title := fields[7]
	aux := ""
	if len(fields) >= 9 {
		aux = fields[8]
	}

	return domain.Timer{
		ID:           timerID,
		Active:       active,
		ChannelID:    fields[1],
		Day:          day,
		Start:        startTime,
		Stop:         stopTime,
		DaySpec:      daySpec,
		StartMinutes: startClock,
		StopMinutes:  stopClock,
		Priority:     priority,
		Lifetime:     lifetime,
		Title:        title,
		Aux:          aux,
	}, nil
}

func isWeekdayMask(daySpec string) bool {
	daySpec = strings.TrimSpace(daySpec)
	if len(daySpec) != 7 {
		return false
	}
	// VDR uses a position-based weekday mask (Mon..Sun) with letters or '-' for disabled days.
	for _, r := range daySpec {
		switch r {
		case 'M', 'T', 'W', 'F', 'S', '-', '.':
			// ok
		default:
			return false
		}
	}
	return true
}

func weekdayMaskAllows(daySpec string, wd time.Weekday) bool {
	// Map Go weekday (Sun=0) to mask index (Mon=0..Sun=6).
	idx := 0
	switch wd {
	case time.Monday:
		idx = 0
	case time.Tuesday:
		idx = 1
	case time.Wednesday:
		idx = 2
	case time.Thursday:
		idx = 3
	case time.Friday:
		idx = 4
	case time.Saturday:
		idx = 5
	case time.Sunday:
		idx = 6
	}
	if idx < 0 || idx >= len(daySpec) {
		return false
	}
	c := daySpec[idx]
	return c != '-' && c != '.'
}

func nextOccurrenceDay(daySpec string, startMinutes int, now time.Time) time.Time {
	loc := time.Local
	localNow := now.In(loc)
	base := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	for i := 0; i < 8; i++ {
		day := base.Add(time.Duration(i) * 24 * time.Hour)
		if !weekdayMaskAllows(daySpec, day.Weekday()) {
			continue
		}
		if startMinutes >= 0 {
			start := day.Add(time.Duration(startMinutes) * time.Minute)
			if !start.After(localNow) {
				continue
			}
		}
		return day
	}
	return base
}

func parseTimerDay(daySpec string) time.Time {
	daySpec = strings.TrimSpace(daySpec)
	if daySpec == "" {
		return time.Time{}
	}

	if t, err := time.ParseInLocation("2006-01-02", daySpec, time.Local); err == nil {
		return t
	}
	if t, err := time.ParseInLocation("20060102", daySpec, time.Local); err == nil {
		return t
	}

	return time.Time{}
}

func parseTimerClock(clock string) int {
	clock = strings.TrimSpace(clock)
	if clock == "" {
		return -1
	}
	if strings.Contains(clock, ":") {
		if t, err := time.Parse("15:04", clock); err == nil {
			return t.Hour()*60 + t.Minute()
		}
		return -1
	}
	if len(clock) != 4 {
		return -1
	}
	hh, err1 := strconv.Atoi(clock[:2])
	mm, err2 := strconv.Atoi(clock[2:])
	if err1 != nil || err2 != nil {
		return -1
	}
	if hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return -1
	}
	return hh*60 + mm
}

func formatTimer(timer *domain.Timer) string {
	active := "0"
	if timer.Active {
		active = "1"
	}

	// SVDRP timer format:
	// active:channel:day:start:stop:priority:lifetime:file:aux
	// IMPORTANT: Using a weekday mask (e.g. MTWTFSS) creates a repeating timer.
	// For "Record" actions from EPG we want a one-time timer for the selected date.

	daySpec := strings.TrimSpace(timer.DaySpec)
	if daySpec == "" || (!isWeekdayMask(daySpec) && parseTimerDay(daySpec).IsZero()) {
		day := timer.Day
		if day.IsZero() {
			day = timer.Start
		}
		if day.IsZero() {
			day = time.Now()
		}
		daySpec = day.In(time.Local).Format("2006-01-02")
	}

	// For one-time timers, Start/Stop timestamps are authoritative.
	// For repeating timers (weekday mask), preserve the raw clock minutes if available.
	startMinutes := -1
	stopMinutes := -1
	if isWeekdayMask(daySpec) {
		if timer.StartMinutes >= 0 {
			startMinutes = timer.StartMinutes
		}
		if timer.StopMinutes >= 0 {
			stopMinutes = timer.StopMinutes
		}
	}
	if startMinutes < 0 {
		if !timer.Start.IsZero() {
			startMinutes = timer.Start.In(time.Local).Hour()*60 + timer.Start.In(time.Local).Minute()
		} else {
			startMinutes = 0
		}
	}
	if stopMinutes < 0 {
		if !timer.Stop.IsZero() {
			stopMinutes = timer.Stop.In(time.Local).Hour()*60 + timer.Stop.In(time.Local).Minute()
		} else {
			stopMinutes = 0
		}
	}

	startClock := fmt.Sprintf("%02d%02d", startMinutes/60, startMinutes%60)
	stopClock := fmt.Sprintf("%02d%02d", stopMinutes/60, stopMinutes%60)

	file := sanitizeTimerField(timer.Title)
	aux := sanitizeTimerField(timer.Aux)

	return fmt.Sprintf("%s:%s:%s:%s:%s:%d:%d:%s:%s",
		active,
		timer.ChannelID,
		daySpec,
		startClock,
		stopClock,
		timer.Priority,
		timer.Lifetime,
		file,
		aux,
	)
}

func sanitizeTimerField(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, ":", "|")
	return strings.TrimSpace(s)
}

func parseRecording(line string) (domain.Recording, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return domain.Recording{}, fmt.Errorf("invalid recording format")
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return domain.Recording{}, fmt.Errorf("invalid recording format")
	}

	// VDR 2.7.x `LSTR` format example (after SVDRP prefix stripping):
	//   "1 02.04.21 15:03 0:22* Title" or "30 08.08.19 22:23 1:37* Folder~Title~Subtitle"
	// Where the first field is the recording index used by `DELR`.
	if _, err := strconv.Atoi(fields[0]); err == nil && len(fields) >= 5 &&
		recordingShortDateRe.MatchString(fields[1]) && recordingClockRe.MatchString(fields[2]) && recordingLengthRe.MatchString(fields[3]) {
		idx := fields[0]
		stamp := fields[1] + " " + fields[2]
		rec := domain.Recording{Path: idx}
		if t, err := time.ParseInLocation("02.01.06 15:04", stamp, time.Local); err == nil {
			rec.Date = t
		}
		rec.Length = parseRecordingLength(fields[3])

		metaText := strings.Join(fields[4:], " ")
		applyRecordingMeta(&rec, metaText)
		return rec, nil
	}

	// Legacy/other variants we’ve seen:
	//   "<id> <path> <meta...>" or "<path> <meta...>"
	path := ""
	infoText := ""
	if _, err := strconv.Atoi(fields[0]); err == nil {
		path = fields[1]
		if len(fields) > 2 {
			infoText = strings.Join(fields[2:], " ")
		}
	} else {
		path = fields[0]
		infoText = strings.Join(fields[1:], " ")
	}

	rec := domain.Recording{Path: path}
	rec.Date = parseRecordingDateFromPath(path)
	if rec.Date.IsZero() {
		rec.Date = parseRecordingDateFromMeta(infoText)
	}
	applyRecordingMeta(&rec, infoText)
	return rec, nil
}

func parseRecordingLength(token string) time.Duration {
	token = strings.TrimSpace(token)
	// Tokens may include trailing flags like '*' and '!' (e.g. "4:22*!" or "0:22*").
	token = strings.TrimRight(token, "*!")
	parts := strings.Split(token, ":")
	if len(parts) != 2 {
		return 0
	}
	hours, err1 := strconv.Atoi(parts[0])
	mins, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0
	}
	if hours < 0 || mins < 0 || mins > 59 {
		return 0
	}
	return time.Duration(hours)*time.Hour + time.Duration(mins)*time.Minute
}

func applyRecordingMeta(rec *domain.Recording, metaText string) {
	metaText = strings.TrimSpace(metaText)
	if metaText == "" {
		return
	}

	// Metadata is often "folder/channel~title~subtitle~description..." (may contain spaces).
	info := strings.Split(metaText, "~")
	for i := range info {
		info[i] = strings.TrimSpace(info[i])
	}
	if len(info) == 1 {
		rec.Title = info[0]
		return
	}

	// If there are multiple fields, VDR often prefixes with a folder/channel grouping.
	rec.Channel = info[0]
	if len(info) > 1 {
		rec.Title = info[1]
	}
	if len(info) > 2 {
		rec.Subtitle = info[2]
	}
	if len(info) > 3 {
		rec.Description = strings.Join(info[3:], "~")
	}
}

func parseRecordingDateFromPath(path string) time.Time {
	// VDR recordings commonly contain a timestamp segment like:
	// 2025-01-02.20.15.50-0.rec
	m := recordingPathTimestampRe.FindStringSubmatch(path)
	if len(m) != 5 {
		return time.Time{}
	}
	stamp := fmt.Sprintf("%s.%s.%s.%s", m[1], m[2], m[3], m[4])
	// Interpret as local time for display consistency.
	if t, err := time.ParseInLocation("2006-01-02.15.04.05", stamp, time.Local); err == nil {
		return t
	}
	return time.Time{}
}

func parseRecordingDateFromMeta(meta string) time.Time {
	meta = strings.TrimSpace(meta)
	if meta == "" {
		return time.Time{}
	}
	// Example: "2025-01-02 20:15~Title..." or "2025-01-02 20:15 Title..."
	m := recordingListLeadingDateTimeRe.FindStringSubmatch(meta)
	if len(m) != 4 {
		return time.Time{}
	}
	stamp := fmt.Sprintf("%s %s:%s", m[1], m[2], m[3])
	if t, err := time.ParseInLocation("2006-01-02 15:04", stamp, time.Local); err == nil {
		return t
	}
	return time.Time{}
}
