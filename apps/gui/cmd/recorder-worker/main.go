package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/checker"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/recorder"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/workerproc"
)

type stdoutNotifier struct {
	mu sync.Mutex
}

func (n *stdoutNotifier) emit(evt workerproc.Event) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if evt.Time == "" {
		evt.Time = time.Now().Format(time.RFC3339)
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	fmt.Println(string(data))
}

func (n *stdoutNotifier) NotifyStatus(screenID, status, message string) {
	n.emit(workerproc.Event{
		Type:     "status",
		ScreenID: screenID,
		Status:   status,
		Message:  message,
	})
}

func (n *stdoutNotifier) NotifyAppLog(message string) {
	n.emit(workerproc.Event{
		Type:    "log",
		Message: message,
	})
}

func loadJob(path string) (*workerproc.Job, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("job path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var job workerproc.Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}
	job.ScreenID = strings.TrimSpace(job.ScreenID)
	job.StreamURL = strings.TrimSpace(job.StreamURL)
	job.Mode = strings.ToLower(strings.TrimSpace(job.Mode))
	if job.Mode == "" {
		job.Mode = "record"
	}
	if job.ScreenID == "" {
		return nil, fmt.Errorf("screen_id is empty")
	}
	if job.Mode != "monitor" && job.StreamURL == "" {
		return nil, fmt.Errorf("stream_url is empty")
	}
	if job.Mode != "record" && job.Mode != "monitor" {
		return nil, fmt.Errorf("unsupported mode: %s", job.Mode)
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
	job.Options.ProxyURL = strings.TrimSpace(job.Options.ProxyURL)
	if job.CheckIntervalSeconds <= 0 {
		job.CheckIntervalSeconds = 30
	}
	if job.CheckIntervalSeconds < 5 {
		job.CheckIntervalSeconds = 5
	}
	return &job, nil
}

func main() {
	jobPath := flag.String("job", "", "Path to recorder worker job JSON")
	flag.Parse()

	notifier := &stdoutNotifier{}
	job, err := loadJob(*jobPath)
	if err != nil {
		notifier.emit(workerproc.Event{Type: "error", Error: err.Error()})
		os.Exit(2)
	}

	stopCh := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func(message string) {
		stopOnce.Do(func() {
			notifier.emit(workerproc.Event{
				Type:     "log",
				ScreenID: job.ScreenID,
				Message:  message,
			})
			close(stopCh)
		})
		_ = recorder.StopRecording(job.ScreenID)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		requestStop("interrupt received, stopping recording")
	}()

	if strings.TrimSpace(job.StopFile) != "" {
		go watchStopFile(job, requestStop)
	}

	notifier.emit(workerproc.Event{
		Type:     "start",
		ScreenID: job.ScreenID,
		Message:  "recorder worker started",
	})
	checker.SetProxyURL(job.Options.ProxyURL)

	var result workerproc.Event
	if job.Mode == "monitor" {
		result = runMonitorJob(job, notifier, stopCh)
	} else {
		result = runRecordJob(job, notifier)
	}
	notifier.emit(result)

	if result.Error != "" && !result.StoppedByUser {
		os.Exit(1)
	}
}

func runRecordJob(job *workerproc.Job, notifier *stdoutNotifier) workerproc.Event {
	auth := recorder.AuthSettings{
		Mode:          job.Auth.Mode,
		AccessToken:   job.Auth.AccessToken,
		CookieEnabled: job.Auth.CookieEnabled,
		CookieFile:    job.Auth.CookieFile,
	}
	options := recorder.RecordOptions{
		QualityMode:          job.Options.QualityMode,
		ContainerMode:        job.Options.ContainerMode,
		SaveInfoText:         job.Options.SaveInfoText,
		SaveCommentsText:     job.Options.SaveCommentsText,
		SaveCommentsTextFile: job.Options.SaveCommentsTextFile,
		CommentTextTemplate:  job.Options.CommentTextTemplate,
		MovieID:              job.MovieID,
		FFmpegPath:           job.Options.FFmpegPath,
		FFprobePath:          job.Options.FFprobePath,
		ProxyURL:             job.Options.ProxyURL,
	}

	var duration, filePath string
	var fileSize int64
	var stoppedByUser bool
	var recErr error
	duration, filePath, fileSize, stoppedByUser, recErr = recorder.RecordLiveStreamWithOptions(job.ScreenID, job.Title, job.StreamURL, job.OutputDir, auth, options, notifier)

	result := workerproc.Event{
		Type:          "result",
		ScreenID:      job.ScreenID,
		Duration:      duration,
		FilePath:      filePath,
		FileSize:      fileSize,
		StoppedByUser: stoppedByUser,
	}
	if recErr != nil {
		result.Error = recErr.Error()
	}
	return result
}

func runMonitorJob(job *workerproc.Job, notifier *stdoutNotifier, stopCh <-chan struct{}) workerproc.Event {
	interval := time.Duration(job.CheckIntervalSeconds) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}

	for {
		select {
		case <-stopCh:
			return workerproc.Event{Type: "result", ScreenID: job.ScreenID, StoppedByUser: true}
		default:
		}

		notifier.NotifyStatus(job.ScreenID, "monitoring", "Worker monitoring for live stream...")
		info, err := checker.CheckStreamStatusWithAuth(job.ScreenID, checker.AuthOptions{
			Mode:          job.Auth.Mode,
			AccessToken:   job.Auth.AccessToken,
			CookieEnabled: job.Auth.CookieEnabled,
			CookieFile:    job.Auth.CookieFile,
		})
		if err != nil {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker check failed: %v", job.ScreenID, err))
			if waitOrStop(interval, stopCh) {
				return workerproc.Event{Type: "result", ScreenID: job.ScreenID, StoppedByUser: true}
			}
			continue
		}

		if info == nil || !info.IsLive {
			if waitOrStop(interval, stopCh) {
				return workerproc.Event{Type: "result", ScreenID: job.ScreenID, StoppedByUser: true}
			}
			continue
		}
		if job.Auth.CookieEnabled && (job.Auth.Mode == "auto" || job.Auth.Mode == "cookie") {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Cookie auth used for live URL resolution", job.ScreenID))
		}
		if !checker.IsRecordableLiveURL(info.StreamURL) {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker ignored live candidate with non-recordable stream URL", job.ScreenID))
			if waitOrStop(interval, stopCh) {
				return workerproc.Event{Type: "result", ScreenID: job.ScreenID, StoppedByUser: true}
			}
			continue
		}

		notifier.NotifyStatus(job.ScreenID, "monitoring", "Worker detected live candidate, confirming...")
		if waitOrStop(5*time.Second, stopCh) {
			return workerproc.Event{Type: "result", ScreenID: job.ScreenID, StoppedByUser: true}
		}
		confirmedInfo, err := checker.CheckStreamStatusWithAuth(job.ScreenID, checker.AuthOptions{
			Mode:          job.Auth.Mode,
			AccessToken:   job.Auth.AccessToken,
			CookieEnabled: job.Auth.CookieEnabled,
			CookieFile:    job.Auth.CookieFile,
		})
		if err != nil {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker second check failed: %v", job.ScreenID, err))
			if waitOrStop(interval, stopCh) {
				return workerproc.Event{Type: "result", ScreenID: job.ScreenID, StoppedByUser: true}
			}
			continue
		}
		if confirmedInfo == nil || !confirmedInfo.IsLive {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker ignored live candidate: second check is offline", job.ScreenID))
			if waitOrStop(interval, stopCh) {
				return workerproc.Event{Type: "result", ScreenID: job.ScreenID, StoppedByUser: true}
			}
			continue
		}
		if !checker.IsRecordableLiveURL(confirmedInfo.StreamURL) {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker ignored live candidate: second stream URL is not recordable", job.ScreenID))
			if waitOrStop(interval, stopCh) {
				return workerproc.Event{Type: "result", ScreenID: job.ScreenID, StoppedByUser: true}
			}
			continue
		}

		recordJob := *job
		recordJob.Title = confirmedInfo.Title
		recordJob.MovieID = confirmedInfo.MovieID
		recordJob.StreamURL = confirmedInfo.StreamURL
		result := runRecordJob(&recordJob, notifier)
		notifier.emit(result)
		if result.Error != "" && !result.StoppedByUser {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker recording failed: %s", job.ScreenID, result.Error))
			if waitOrStop(3*time.Second, stopCh) {
				result.StoppedByUser = true
				return result
			}
			continue
		}
		if result.StoppedByUser {
			return result
		}

		notifier.NotifyStatus(job.ScreenID, "monitoring", "Recording finished, worker waiting for next stream")
	}
}

func waitOrStop(d time.Duration, stopCh <-chan struct{}) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-stopCh:
		return true
	case <-timer.C:
		return false
	}
}

func watchStopFile(job *workerproc.Job, requestStop func(string)) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		if _, err := os.Stat(job.StopFile); err != nil {
			continue
		}
		requestStop("stop file detected, stopping recording")
		return
	}
}
