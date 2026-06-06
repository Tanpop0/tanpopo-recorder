package workerproc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type EventHandler func(Event)

type Manager struct {
	WorkerPath string
	WorkDir    string
	OnEvent    EventHandler
}

type Handle struct {
	cmd      *exec.Cmd
	jobPath  string
	stopFile string
	done     chan error
	once     sync.Once
}

func (m *Manager) Start(ctx context.Context, job Job) (*Handle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	job = normalizeJob(job)
	if job.ScreenID == "" {
		return nil, fmt.Errorf("screen_id is empty")
	}
	if job.Mode != "monitor" && job.StreamURL == "" {
		return nil, fmt.Errorf("stream_url is empty")
	}
	if job.Mode != "record" && job.Mode != "monitor" {
		return nil, fmt.Errorf("unsupported worker mode: %s", job.Mode)
	}

	workerPath, err := m.resolveWorkerPath()
	if err != nil {
		return nil, err
	}

	workDir := strings.TrimSpace(m.WorkDir)
	if workDir == "" {
		workDir, err = os.MkdirTemp("", "twitcasting-worker-*")
		if err != nil {
			return nil, err
		}
	} else if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, err
	}

	jobPath := filepath.Join(workDir, sanitizePathPart(job.ScreenID)+"-"+time.Now().Format("20060102-150405")+".json")
	if strings.TrimSpace(job.StopFile) == "" {
		job.StopFile = filepath.Join(workDir, sanitizePathPart(job.ScreenID)+".stop")
	}
	_ = os.Remove(job.StopFile)

	if err := writeJobFile(jobPath, job); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, workerPath, "--job", jobPath)
	prepareCommand(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	h := &Handle{
		cmd:      cmd,
		jobPath:  jobPath,
		stopFile: job.StopFile,
		done:     make(chan error, 1),
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go scanWorkerStdout(stdout, m.OnEvent)
	go scanWorkerStderr(stderr, m.OnEvent)
	go func() {
		err := cmd.Wait()
		h.done <- err
		close(h.done)
	}()

	return h, nil
}

func (m *Manager) resolveWorkerPath() (string, error) {
	if strings.TrimSpace(m.WorkerPath) != "" {
		if _, err := os.Stat(m.WorkerPath); err == nil {
			return m.WorkerPath, nil
		}
		return "", fmt.Errorf("worker executable not found: %s", m.WorkerPath)
	}

	candidates := make([]string, 0, 4)
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, exe)
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "recorder-worker.exe"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "recorder-worker.exe"))
		candidates = append(candidates, filepath.Join(cwd, "cmd", "recorder-worker", "recorder-worker.exe"))
	}
	if path, err := exec.LookPath("recorder-worker.exe"); err == nil {
		candidates = append(candidates, path)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("embedded worker or recorder-worker.exe not found")
}

func (h *Handle) Stop(timeout time.Duration) error {
	if h == nil || h.cmd == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	var writeErr error
	h.once.Do(func() {
		writeErr = os.WriteFile(h.stopFile, []byte(time.Now().Format(time.RFC3339)), 0644)
	})
	if writeErr != nil {
		return writeErr
	}

	select {
	case err := <-h.done:
		return err
	case <-time.After(timeout):
		if h.cmd.Process != nil {
			return h.cmd.Process.Kill()
		}
	}
	return nil
}

func (h *Handle) Wait() error {
	if h == nil {
		return nil
	}
	err, ok := <-h.done
	if !ok {
		return nil
	}
	return err
}

func normalizeJob(job Job) Job {
	job.Mode = strings.ToLower(strings.TrimSpace(job.Mode))
	job.ScreenID = strings.TrimSpace(job.ScreenID)
	job.StreamURL = strings.TrimSpace(job.StreamURL)
	if job.Mode == "" {
		job.Mode = "record"
	}
	if strings.TrimSpace(job.Title) == "" {
		job.Title = "untitled"
	}
	if strings.TrimSpace(job.OutputDir) == "" {
		job.OutputDir = "."
	}
	if strings.TrimSpace(job.Auth.Mode) == "" {
		job.Auth.Mode = "auto"
	}
	if strings.TrimSpace(job.Auth.CookieFile) == "" {
		job.Auth.CookieFile = "cookies.txt"
	}
	if strings.TrimSpace(job.Options.QualityMode) == "" {
		job.Options.QualityMode = "stable"
	}
	if strings.TrimSpace(job.Options.ContainerMode) == "" {
		job.Options.ContainerMode = "mkv"
	}
	if job.CheckIntervalSeconds <= 0 {
		job.CheckIntervalSeconds = 30
	}
	if job.CheckIntervalSeconds < 5 {
		job.CheckIntervalSeconds = 5
	}
	if job.CheckIntervalSeconds > 300 {
		job.CheckIntervalSeconds = 300
	}
	return job
}

func writeJobFile(path string, job Job) error {
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func scanWorkerStdout(stdout interface {
	Read([]byte) (int, error)
}, onEvent EventHandler) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		evt, err := parseEventLine(scanner.Text())
		if err != nil {
			if onEvent != nil {
				onEvent(Event{Type: "log", Message: scanner.Text()})
			}
			continue
		}
		if onEvent != nil {
			onEvent(evt)
		}
	}
}

func scanWorkerStderr(stderr interface {
	Read([]byte) (int, error)
}, onEvent EventHandler) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		if onEvent != nil {
			onEvent(Event{Type: "log", Message: scanner.Text()})
		}
	}
}

func parseEventLine(line string) (Event, error) {
	var evt Event
	if err := json.Unmarshal([]byte(line), &evt); err != nil {
		return Event{}, err
	}
	if strings.TrimSpace(evt.Type) == "" {
		return Event{}, fmt.Errorf("worker event type is empty")
	}
	return evt, nil
}

func sanitizePathPart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "worker"
	}
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(s)
}
