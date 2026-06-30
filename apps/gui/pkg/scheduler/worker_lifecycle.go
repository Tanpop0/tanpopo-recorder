package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/checker"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/workerproc"
)

func (m *ValidationManager) startWorkerMonitor(screenID string) {
	if strings.TrimSpace(screenID) == "" {
		return
	}
	if _, paused := m.pausedStreamers.Load(screenID); paused {
		return
	}
	if _, exists := m.workerHandles.Load(screenID); exists {
		return
	}

	auth := m.getAuthConfigForStreamer(screenID)
	recording := m.getRecordingConfigForStreamer(screenID)
	checker.SetProxyURL(recording.proxyURL())
	job := workerproc.Job{
		Mode:                 "monitor",
		ScreenID:             screenID,
		OutputDir:            m.getOutputDirectory(),
		CheckIntervalSeconds: recording.WorkerCheckIntervalSeconds,
		Auth: workerproc.AuthSettings{
			Mode:          auth.Mode,
			AccessToken:   auth.AccessToken,
			CookieEnabled: auth.CookieEnabled,
			CookieFile:    auth.CookieFile,
		},
		Options: workerproc.RecordOptions{
			QualityMode:          recording.QualityMode,
			ContainerMode:        recording.ContainerMode,
			SaveInfoText:         recording.SaveInfoText,
			SaveCommentsText:     recording.SaveCommentsText,
			SaveCommentsTextFile: recording.SaveCommentsTextFile,
			CommentTextTemplate:  recording.CommentTextTemplate,
			FFmpegPath:           recording.FFmpegPath,
			FFprobePath:          recording.FFprobePath,
			ProxyURL:             recording.proxyURL(),
		},
	}

	manager := &workerproc.Manager{
		WorkerPath: recording.WorkerPath,
		OnEvent: func(evt workerproc.Event) {
			m.handleWorkerEvent(screenID, evt)
		},
	}

	handle, err := manager.Start(context.Background(), job)
	if err != nil {
		if m.notifier != nil {
			m.notifier.NotifyStatus(screenID, "error", fmt.Sprintf("Worker start failed: %v", err))
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker start failed: %v", screenID, err))
		}
		return
	}

	if _, loaded := m.workerHandles.LoadOrStore(screenID, handle); loaded {
		_ = handle.Stop(2 * time.Second)
		return
	}

	if m.notifier != nil {
		m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker monitor process started", screenID))
	}

	go func() {
		err := handle.Wait()
		m.workerHandles.Delete(screenID)
		m.activeRecordings.Delete(screenID)
		if _, paused := m.pausedStreamers.Load(screenID); paused {
			return
		}
		if err != nil {
			if m.notifier != nil {
				m.notifier.NotifyStatus(screenID, "error", fmt.Sprintf("Worker exited: %v", err))
				m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker exited: %v", screenID, err))
			}
			m.scheduleWorkerRestart(screenID, err)
			return
		}
		if m.notifier != nil {
			m.notifier.NotifyStatus(screenID, "monitoring", "Worker monitor stopped")
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker monitor stopped", screenID))
		}
		m.scheduleWorkerRestart(screenID, nil)
	}()
}

func (m *ValidationManager) stopWorkerMonitor(screenID string) {
	value, ok := m.workerHandles.LoadAndDelete(screenID)
	if !ok {
		return
	}
	handle, ok := value.(*workerproc.Handle)
	if !ok {
		return
	}
	if err := handle.Stop(10 * time.Second); err != nil && m.notifier != nil {
		m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Stop worker failed: %v", screenID, err))
	}
	m.activeRecordings.Delete(screenID)
	m.workerRestarts.Delete(screenID)
}

func (m *ValidationManager) scheduleWorkerRestart(screenID string, exitErr error) {
	if _, paused := m.pausedStreamers.Load(screenID); paused {
		return
	}
	if !m.getRecordingConfig().WorkerEnabled {
		return
	}
	if _, exists := m.workerHandles.Load(screenID); exists {
		return
	}

	attempt := 0
	if value, ok := m.workerRestarts.Load(screenID); ok {
		if n, okCast := value.(int); okCast {
			attempt = n
		}
	}
	recording := m.getRecordingConfig()
	if recording.WorkerMaxRestarts > 0 && attempt >= recording.WorkerMaxRestarts {
		m.pausedStreamers.Store(screenID, true)
		m.workerRestarts.Delete(screenID)
		if m.notifier != nil {
			m.notifier.NotifyStatus(screenID, "error", fmt.Sprintf("Worker stopped after %d failed restarts", attempt))
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker restart fuse opened after %d failed restarts", screenID, attempt))
		}
		return
	}

	delay := 5 * time.Second
	switch {
	case attempt >= 6:
		delay = 60 * time.Second
	case attempt >= 3:
		delay = 30 * time.Second
	case attempt >= 1:
		delay = 15 * time.Second
	}
	m.workerRestarts.Store(screenID, attempt+1)

	if m.notifier != nil {
		reason := "worker exited"
		if exitErr != nil {
			reason = exitErr.Error()
		}
		m.notifier.NotifyStatus(screenID, "monitoring", fmt.Sprintf("Worker restarting in %s", delay))
		m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker restart scheduled in %s (%s)", screenID, delay, reason))
	}

	go func() {
		time.Sleep(delay)
		if _, paused := m.pausedStreamers.Load(screenID); paused {
			return
		}
		if _, exists := m.workerHandles.Load(screenID); exists {
			return
		}
		m.startWorkerMonitor(screenID)
	}()
}

func (m *ValidationManager) handleWorkerEvent(fallbackScreenID string, evt workerproc.Event) {
	screenID := strings.TrimSpace(evt.ScreenID)
	if screenID == "" {
		screenID = fallbackScreenID
	}
	_, paused := m.pausedStreamers.Load(screenID)
	if paused && evt.Type != "result" {
		return
	}

	switch evt.Type {
	case "start":
		if m.notifier != nil {
			message := evt.Message
			if message == "" {
				message = "Worker started"
			}
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] %s", screenID, message))
		}
	case "status":
		if evt.Status == "recording" {
			_, alreadyRecording := m.activeRecordings.Load(screenID)
			m.activeRecordings.Store(screenID, true)
			m.workerRestarts.Delete(screenID)
			if !alreadyRecording {
				m.sendRecordingNotification("start", screenID, m.recordingStartMessage(screenID, ""))
			}
		} else if evt.Status != "" {
			m.activeRecordings.Delete(screenID)
		}
		if m.notifier != nil {
			m.notifier.NotifyStatus(screenID, evt.Status, evt.Message)
		}
	case "log":
		if m.notifier != nil && strings.TrimSpace(evt.Message) != "" {
			m.notifier.NotifyAppLog(evt.Message)
		}
	case "result":
		m.activeRecordings.Delete(screenID)
		if evt.Error == "" && !evt.StoppedByUser {
			m.workerRestarts.Delete(screenID)
		}
		if evt.FilePath != "" && m.notifier != nil && (evt.FileSize > 0 || evt.StoppedByUser) {
			recErr := error(nil)
			if evt.Error != "" {
				recErr = fmt.Errorf("%s", evt.Error)
			}
			historyStatus := classifyRecordingStatus(evt.Duration, evt.FileSize, evt.StoppedByUser, recErr, m.getRecordingConfigForStreamer(screenID))
			m.notifier.AddRecordingHistoryWithStatus(screenID, evt.FilePath, evt.Duration, evt.FileSize, historyStatus)
		}
		if evt.Error != "" && !evt.StoppedByUser && m.notifier != nil {
			if !paused {
				m.notifier.NotifyStatus(screenID, "error", fmt.Sprintf("Recording failed: %v", evt.Error))
				m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Recording failed: %v", screenID, evt.Error))
			}
		}
		if paused {
			return
		}
		if evt.Error != "" && !evt.StoppedByUser {
			m.sendRecordingNotification("error", screenID, m.recordingErrorMessage(screenID, "", evt.Error))
		} else if evt.Error == "" && !evt.StoppedByUser {
			m.sendRecordingNotification("finish", screenID, m.recordingFinishMessage(screenID, "", evt.Duration, evt.FilePath, evt.FileSize))
		}
	case "error":
		if m.notifier != nil {
			m.notifier.NotifyStatus(screenID, "error", evt.Error)
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Worker error: %s", screenID, evt.Error))
		}
		m.sendRecordingNotification("error", screenID, m.workerErrorMessage(screenID, evt.Error))
	default:
		if m.notifier != nil {
			data := strings.TrimSpace(evt.Message)
			if data == "" {
				data = fmt.Sprintf("Worker event: %s", evt.Type)
			}
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] %s", screenID, data))
		}
	}
}
