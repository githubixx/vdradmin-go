package archive

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Kind string

const (
	KindMovie  Kind = "movie"
	KindSeries Kind = "series"
)

type ParsedInfo struct {
	Title   string
	Episode string
	Kind    Kind
}

func ParseVDRInfo(r io.Reader) (ParsedInfo, error) {
	scanner := bufio.NewScanner(r)
	var title string
	var episode string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.HasPrefix(line, "T ") {
			if title == "" {
				title = strings.TrimSpace(strings.TrimPrefix(line, "T "))
			}
			continue
		}
		if strings.HasPrefix(line, "S ") {
			if episode == "" {
				episode = strings.TrimSpace(strings.TrimPrefix(line, "S "))
			}
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return ParsedInfo{}, fmt.Errorf("read info: %w", err)
	}
	if strings.TrimSpace(title) == "" {
		return ParsedInfo{}, errors.New("missing T line in info")
	}
	kind := KindMovie
	if strings.TrimSpace(episode) != "" {
		kind = KindSeries
	}
	return ParsedInfo{Title: title, Episode: episode, Kind: kind}, nil
}

type ArchiveProfile struct {
	ID      string
	Name    string
	Kind    Kind
	BaseDir string
}

func DefaultProfiles(archiveBaseDir string) []ArchiveProfile {
	base := strings.TrimSpace(archiveBaseDir)
	return []ArchiveProfile{
		{
			ID:      "movies",
			Name:    "Movies",
			Kind:    KindMovie,
			BaseDir: filepath.Join(base, "movies"),
		},
		{
			ID:      "series",
			Name:    "Series",
			Kind:    KindSeries,
			BaseDir: filepath.Join(base, "series"),
		},
	}
}

func FindProfile(profiles []ArchiveProfile, id string) (ArchiveProfile, bool) {
	for _, p := range profiles {
		if p.ID == id {
			return p, true
		}
	}
	return ArchiveProfile{}, false
}

func DefaultProfileIDForKind(k Kind) string {
	if k == KindSeries {
		return "series"
	}
	return "movies"
}

func Slugify(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	repl := strings.NewReplacer(
		"ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss",
		"Ä", "ae", "Ö", "oe", "Ü", "ue",
	)
	s = repl.Replace(s)
	s = strings.ToLower(s)

	// Convert to allowed charset: [a-z0-9_-], others -> '_'
	var b strings.Builder
	b.Grow(len(s))
	lastUnderscore := false
	for _, r := range s {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if isAllowed {
			b.WriteRune(r)
			lastUnderscore = (r == '_')
			continue
		}
		if r == ' ' || r == '\t' {
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := b.String()
	out = strings.Trim(out, "_")
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return out
}

type Preview struct {
	TargetDir   string
	VideoPath   string
	InfoDstPath string
	TitleSlug   string
	EpisodeSlug string
}

func normalizeVideoExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	ext = strings.TrimPrefix(ext, ".")
	if ext == "mp4" {
		return "mp4"
	}
	// Default/fallback.
	return "mkv"
}

// NormalizePreview fills missing derived paths and validates that
// VideoPath/InfoDstPath live inside TargetDir.
//
// This is useful for "Profile=None" workflows where the user provides
// a custom target directory or output paths.
func NormalizePreview(p Preview, videoExt string) (Preview, error) {
	videoExt = normalizeVideoExt(videoExt)
	targetDir := strings.TrimSpace(p.TargetDir)
	videoPath := strings.TrimSpace(p.VideoPath)
	infoDstPath := strings.TrimSpace(p.InfoDstPath)

	if targetDir == "" && videoPath != "" {
		targetDir = filepath.Dir(videoPath)
	}
	if targetDir == "" {
		return Preview{}, errors.New("target_dir is required")
	}
	targetDir = filepath.Clean(targetDir)

	if videoPath == "" {
		videoPath = filepath.Join(targetDir, "video."+videoExt)
	}
	if infoDstPath == "" {
		infoDstPath = filepath.Join(targetDir, "video.info")
	}

	if filepath.Clean(filepath.Dir(videoPath)) != targetDir {
		return Preview{}, errors.New("video_path must be inside target_dir")
	}
	if filepath.Clean(filepath.Dir(infoDstPath)) != targetDir {
		return Preview{}, errors.New("info_dst_path must be inside target_dir")
	}

	p.TargetDir = targetDir
	p.VideoPath = videoPath
	p.InfoDstPath = infoDstPath
	return p, nil
}

func BuildPreview(profile ArchiveProfile, title string, episode string, videoExt string) (Preview, error) {
	videoExt = normalizeVideoExt(videoExt)
	title = strings.TrimSpace(title)
	episode = strings.TrimSpace(episode)
	if title == "" {
		return Preview{}, errors.New("title is required")
	}
	tSlug := Slugify(title)
	eSlug := Slugify(episode)
	if tSlug == "" {
		return Preview{}, errors.New("title results in empty slug")
	}

	targetDir := ""
	switch profile.Kind {
	case KindMovie:
		targetDir = filepath.Join(profile.BaseDir, tSlug)
	case KindSeries:
		if eSlug == "" {
			// Allow empty episode slug (user can fill it), but keep directory stable.
			targetDir = filepath.Join(profile.BaseDir, tSlug)
		} else {
			targetDir = filepath.Join(profile.BaseDir, tSlug, eSlug)
		}
	default:
		return Preview{}, fmt.Errorf("unknown profile kind: %q", profile.Kind)
	}

	return Preview{
		TargetDir:   targetDir,
		VideoPath:   filepath.Join(targetDir, "video."+videoExt),
		InfoDstPath: filepath.Join(targetDir, "video.info"),
		TitleSlug:   tSlug,
		EpisodeSlug: eSlug,
	}, nil
}

type Plan struct {
	RecordingID  string
	RecordingDir string
	InfoPath     string
	Segments     []string
	ConcatList   string

	Profile ArchiveProfile
	Preview Preview

	FFMpegArgs []string
}

// DiscoverSegments finds and sorts *.ts segments inside a recording directory.
func DiscoverSegments(recordingDir string) ([]string, error) {
	recordingDir = strings.TrimSpace(recordingDir)
	if recordingDir == "" {
		return nil, errors.New("recordingDir is required")
	}
	entries, err := os.ReadDir(recordingDir)
	if err != nil {
		return nil, fmt.Errorf("read recording dir: %w", err)
	}
	segs := make([]string, 0, 16)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".ts") {
			segs = append(segs, filepath.Join(recordingDir, name))
		}
	}
	sort.Slice(segs, func(i, j int) bool {
		return filepath.Base(segs[i]) < filepath.Base(segs[j])
	})
	if len(segs) == 0 {
		return nil, errors.New("no .ts segments found")
	}
	return segs, nil
}

func escapeConcatPath(p string) string {
	// ffmpeg concat demuxer uses single-quoted strings. To include a literal single
	// quote, use the pattern '\'' (close quote, escaped quote, open quote).
	// Backslashes are literal inside single quotes (no escaping needed).
	return strings.ReplaceAll(p, "'", "'\\''")
}

// WriteConcatList writes a concat demuxer list file and returns its path.
// The caller is responsible for deleting the file.
func WriteConcatList(dir string, segments []string) (string, error) {
	if len(segments) == 0 {
		return "", errors.New("segments required")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir concat dir: %w", err)
	}
	f, err := os.CreateTemp(dir, "vdradmin-archive-*.concat")
	if err != nil {
		return "", fmt.Errorf("create concat list: %w", err)
	}
	defer func() { _ = f.Close() }()
	for _, s := range segments {
		line := fmt.Sprintf("file '%s'\n", escapeConcatPath(s))
		if _, err := f.WriteString(line); err != nil {
			_ = os.Remove(f.Name())
			return "", fmt.Errorf("write concat list: %w", err)
		}
	}
	if err := f.Sync(); err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("sync concat list: %w", err)
	}
	return f.Name(), nil
}

func SplitArgs(s string) []string {
	// Minimal shell-like splitting: space-separated, no quoting.
	// Good enough for our config default; can be improved later.
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func BuildPlan(recordingID string, recordingDir string, infoPath string, profile ArchiveProfile, title string, episode string, videoExt string, ffmpegArgs []string) (Plan, error) {
	preview, err := BuildPreview(profile, title, episode, videoExt)
	if err != nil {
		return Plan{}, err
	}
	segs, err := DiscoverSegments(recordingDir)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		RecordingID:  recordingID,
		RecordingDir: recordingDir,
		InfoPath:     infoPath,
		Segments:     segs,
		Profile:      profile,
		Preview:      preview,
		FFMpegArgs:   append([]string(nil), ffmpegArgs...),
	}, nil
}

// BuildPlanWithPreview builds a plan using an explicitly provided Preview.
// It will fill missing file paths from TargetDir and validate that output
// files live inside TargetDir.
func BuildPlanWithPreview(recordingID string, recordingDir string, infoPath string, profile ArchiveProfile, preview Preview, videoExt string, ffmpegArgs []string) (Plan, error) {
	if strings.TrimSpace(recordingID) == "" {
		return Plan{}, errors.New("recordingID is required")
	}
	if strings.TrimSpace(recordingDir) == "" {
		return Plan{}, errors.New("recordingDir is required")
	}
	if _, err := NormalizePreview(preview, videoExt); err != nil {
		return Plan{}, err
	}
	preview, _ = NormalizePreview(preview, videoExt)

	segs, err := DiscoverSegments(recordingDir)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		RecordingID:  recordingID,
		RecordingDir: recordingDir,
		InfoPath:     infoPath,
		Segments:     segs,
		Profile:      profile,
		Preview:      preview,
		FFMpegArgs:   append([]string(nil), ffmpegArgs...),
	}, nil
}

type JobStatus string

const (
	JobQueued  JobStatus = "queued"
	JobRunning JobStatus = "running"
	JobSuccess JobStatus = "success"
	JobFailed  JobStatus = "failed"
)

type Progress struct {
	KnownDuration bool
	Percent       float64
	OutTimeMS     int64
	Speed         string
	Raw           map[string]string
}

type JobSnapshot struct {
	ID          string
	InstanceID  string
	RecordingID string
	Status      JobStatus
	CreatedAt   time.Time
	StartedAt   time.Time
	EndedAt     time.Time
	Error       string
	Preview     Preview
	Progress    Progress
	LogCount    int
	LogTail     string
}

type Job struct {
	mu          sync.RWMutex
	id          string
	instanceID  string
	recordingID string
	status      JobStatus
	created     time.Time
	started     time.Time
	ended       time.Time
	errMsg      string
	preview     Preview
	progress    Progress
	logLines    []string
	cancel      context.CancelFunc
}

func (j *Job) snapshot() JobSnapshot {
	j.mu.RLock()
	defer j.mu.RUnlock()
	max := 120
	start := 0
	if len(j.logLines) > max {
		start = len(j.logLines) - max
	}
	return JobSnapshot{
		ID:          j.id,
		InstanceID:  j.instanceID,
		RecordingID: j.recordingID,
		Status:      j.status,
		CreatedAt:   j.created,
		StartedAt:   j.started,
		EndedAt:     j.ended,
		Error:       j.errMsg,
		Preview:     j.preview,
		Progress:    j.progress,
		LogCount:    len(j.logLines),
		LogTail:     strings.Join(j.logLines[start:], "\n"),
	}
}

// ActiveJobIDForRecording returns the job ID for a currently active (queued/running)
// archive job for the given recording ID.
func (m *JobManager) ActiveJobIDForRecording(recordingID string) (string, bool) {
	recordingID = strings.TrimSpace(recordingID)
	if recordingID == "" {
		return "", false
	}

	// Copy pointers first to avoid holding m.mu while inspecting job internals.
	m.mu.RLock()
	jobs := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	m.mu.RUnlock()

	var bestID string
	var bestCreated time.Time
	for _, j := range jobs {
		if j == nil {
			continue
		}
		snap := j.snapshot()
		if snap.RecordingID != recordingID {
			continue
		}
		if snap.Status != JobQueued && snap.Status != JobRunning {
			continue
		}
		if bestID == "" || snap.CreatedAt.After(bestCreated) {
			bestID = snap.ID
			bestCreated = snap.CreatedAt
		}
	}
	if bestID == "" {
		return "", false
	}
	return bestID, true
}

// ActiveJobIDsByRecording returns a map of recordingID -> jobID for all currently
// active (queued/running) archive jobs.
func (m *JobManager) ActiveJobIDsByRecording() map[string]string {
	// Copy pointers first to avoid holding m.mu while inspecting job internals.
	m.mu.RLock()
	jobs := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	m.mu.RUnlock()

	type best struct {
		id      string
		created time.Time
	}
	bestByRec := map[string]best{}
	for _, j := range jobs {
		if j == nil {
			continue
		}
		snap := j.snapshot()
		if strings.TrimSpace(snap.RecordingID) == "" {
			continue
		}
		if snap.Status != JobQueued && snap.Status != JobRunning {
			continue
		}
		cur, ok := bestByRec[snap.RecordingID]
		if !ok || snap.CreatedAt.After(cur.created) {
			bestByRec[snap.RecordingID] = best{id: snap.ID, created: snap.CreatedAt}
		}
	}

	out := make(map[string]string, len(bestByRec))
	for recID, b := range bestByRec {
		out[recID] = b.id
	}
	return out
}

func (j *Job) logsSince(from int) (lines []string, next int) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if from < 0 {
		from = 0
	}
	if from >= len(j.logLines) {
		return nil, len(j.logLines)
	}
	out := append([]string(nil), j.logLines[from:]...)
	return out, len(j.logLines)
}

func (j *Job) addLog(line string) {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return
	}
	j.mu.Lock()
	j.logLines = append(j.logLines, line)
	j.mu.Unlock()
}

type JobManager struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

func NewJobManager() *JobManager {
	return &JobManager{jobs: make(map[string]*Job)}
}

func (m *JobManager) Count() int {
	m.mu.RLock()
	n := len(m.jobs)
	m.mu.RUnlock()
	return n
}

func (m *JobManager) Get(id string) (JobSnapshot, bool) {
	m.mu.RLock()
	j := m.jobs[id]
	m.mu.RUnlock()
	if j == nil {
		return JobSnapshot{}, false
	}
	return j.snapshot(), true
}

// Poll returns the current snapshot plus log lines since the given offset.
// Use the returned next offset for the subsequent poll.
func (m *JobManager) Poll(id string, from int) (snap JobSnapshot, newLines []string, next int, ok bool) {
	m.mu.RLock()
	j := m.jobs[id]
	m.mu.RUnlock()
	if j == nil {
		return JobSnapshot{}, nil, 0, false
	}
	snap = j.snapshot()
	newLines, next = j.logsSince(from)
	return snap, newLines, next, true
}

func (m *JobManager) List() []JobSnapshot {
	m.mu.RLock()
	snaps := make([]JobSnapshot, 0, len(m.jobs))
	for _, j := range m.jobs {
		snaps = append(snaps, j.snapshot())
	}
	m.mu.RUnlock()
	sort.Slice(snaps, func(i, j int) bool {
		ai := snaps[i].CreatedAt
		aj := snaps[j].CreatedAt
		if ai.IsZero() {
			ai = snaps[i].StartedAt
		}
		if aj.IsZero() {
			aj = snaps[j].StartedAt
		}
		return ai.After(aj)
	})
	return snaps
}

func (m *JobManager) Cancel(id string) bool {
	m.mu.RLock()
	j := m.jobs[id]
	m.mu.RUnlock()
	if j == nil {
		return false
	}
	j.mu.Lock()
	cancel := j.cancel
	status := j.status
	j.mu.Unlock()
	if cancel == nil {
		return false
	}
	if status != JobQueued && status != JobRunning {
		return false
	}
	cancel()
	return true
}

func (m *JobManager) Start(ctx context.Context, plan Plan, instanceID string) (string, error) {
	if plan.Preview.TargetDir == "" {
		return "", errors.New("invalid plan")
	}
	inst := strings.TrimSpace(instanceID)
	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	if inst != "" {
		jobID = inst + "-" + jobID
	}
	ctxRun, cancel := context.WithCancel(ctx)
	j := &Job{id: jobID, instanceID: inst, recordingID: strings.TrimSpace(plan.RecordingID), status: JobQueued, created: time.Now(), preview: plan.Preview, progress: Progress{Raw: map[string]string{}}, cancel: cancel}
	m.mu.Lock()
	m.jobs[jobID] = j
	m.mu.Unlock()

	go func() {
		j.mu.Lock()
		j.status = JobRunning
		j.started = time.Now()
		j.mu.Unlock()

		err := runArchive(ctxRun, j, plan)
		j.mu.Lock()
		defer j.mu.Unlock()
		j.ended = time.Now()
		if err != nil {
			j.status = JobFailed
			if errors.Is(err, context.Canceled) {
				j.errMsg = "canceled"
			} else {
				j.errMsg = err.Error()
			}
		} else {
			j.status = JobSuccess
		}
	}()

	return jobID, nil
}

func ffprobeDurationSeconds(ctx context.Context, concatList string) (float64, error) {
	// Try to compute duration via ffprobe; may not be available.
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-f", "concat",
		"-safe", "0",
		"-i", concatList,
		"-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1",
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, errors.New("empty ffprobe output")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if v <= 0 {
		return 0, errors.New("non-positive duration")
	}
	return v, nil
}

func parseProgressLine(kv string) (key string, val string, ok bool) {
	kv = strings.TrimSpace(kv)
	if kv == "" {
		return "", "", false
	}
	idx := strings.IndexByte(kv, '=')
	if idx <= 0 {
		return "", "", false
	}
	return kv[:idx], kv[idx+1:], true
}

func runArchive(ctx context.Context, job *Job, plan Plan) error {
	// If the job was canceled before the runner starts, avoid touching the filesystem.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Ensure target dir exists.
	if err := os.MkdirAll(plan.Preview.TargetDir, 0755); err != nil {
		return fmt.Errorf("mkdir target dir: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	finalOut := plan.Preview.VideoPath
	// Keep a standard container extension at the end so ffmpeg can infer the muxer.
	// Example: video.mkv -> video.tmp.mkv (instead of video.mkv.tmp which breaks format detection).
	ext := filepath.Ext(finalOut)
	base := strings.TrimSuffix(finalOut, ext)
	var tmpOut string
	if ext == "" {
		tmpOut = finalOut + ".tmp"
	} else {
		tmpOut = base + ".tmp" + ext
	}
	if _, err := os.Stat(finalOut); err == nil {
		return fmt.Errorf("output already exists: %s", finalOut)
	}
	if _, err := os.Stat(tmpOut); err == nil {
		return fmt.Errorf("temp output already exists: %s", tmpOut)
	}

	// Create concat list file in the target dir so paths are easy to diagnose.
	concatList, err := WriteConcatList(plan.Preview.TargetDir, plan.Segments)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(concatList) }()
	if err := ctx.Err(); err != nil {
		return err
	}

	// Best-effort duration probe (for percentage).
	if dur, err := ffprobeDurationSeconds(ctx, concatList); err == nil {
		job.mu.Lock()
		job.progress.KnownDuration = true
		job.progress.Raw["duration_seconds"] = fmt.Sprintf("%.3f", dur)
		job.mu.Unlock()
	}

	args := []string{
		"-hide_banner",
		"-nostats",
		"-progress", "pipe:1",
		"-f", "concat",
		"-safe", "0",
		"-i", concatList,
	}
	args = append(args, plan.FFMpegArgs...)
	args = append(args, tmpOut)

	job.addLog("ffmpeg " + strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Parse progress from stdout.
	progressDone := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			k, v, ok := parseProgressLine(scanner.Text())
			if !ok {
				continue
			}
			job.mu.Lock()
			if job.progress.Raw == nil {
				job.progress.Raw = map[string]string{}
			}
			job.progress.Raw[k] = v
			if k == "out_time_ms" {
				if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
					job.progress.OutTimeMS = n
					if job.progress.KnownDuration {
						if ds, ok := job.progress.Raw["duration_seconds"]; ok {
							if dur, err := strconv.ParseFloat(ds, 64); err == nil && dur > 0 {
								pct := (float64(n) / 1_000_000.0) / dur * 100.0
								if pct < 0 {
									pct = 0
								}
								if pct > 100 {
									pct = 100
								}
								job.progress.Percent = pct
							}
						}
					}
				}
			}
			if k == "speed" {
				job.progress.Speed = v
			}
			job.mu.Unlock()
		}
		close(progressDone)
	}()

	// Collect stderr as log output.
	stderrDone := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			job.addLog(scanner.Text())
		}
		close(stderrDone)
	}()

	// Wait for ffmpeg.
	err = cmd.Wait()
	<-progressDone
	<-stderrDone
	if err != nil {
		_ = os.Remove(tmpOut)
		return fmt.Errorf("ffmpeg failed: %w", err)
	}

	// Atomic-ish: rename tmp output to final.
	if err := os.Rename(tmpOut, finalOut); err != nil {
		return fmt.Errorf("rename output: %w", err)
	}

	// Copy info file.
	if strings.TrimSpace(plan.InfoPath) != "" {
		b, err := os.ReadFile(plan.InfoPath)
		if err == nil {
			_ = os.WriteFile(plan.Preview.InfoDstPath, b, 0644)
		}
	}

	return nil
}
