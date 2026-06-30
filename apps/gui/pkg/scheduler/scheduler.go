package scheduler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/checker"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/notify"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/paths"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/recorder"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/workerproc"

	"github.com/robfig/cron/v3"
)

const (
	liveConfirmDelay                   = 5 * time.Second
	forcedCookieCheckFailureThreshold  = 3
	forcedCookieStatusCheckJitterLimit = 12 * time.Second
)

type StatusNotifier interface {
	NotifyStatus(screenID, status, message string)
	NotifyAppLog(message string)
	AddRecordingHistory(streamerID, filePath, duration string, fileSize int64)
	AddRecordingHistoryWithStatus(streamerID, filePath, duration string, fileSize int64, status string)
}

type AuthConfig struct {
	Mode          string
	AccessToken   string
	CookieEnabled bool
	CookieFile    string
}

func (a AuthConfig) checkerOptions() checker.AuthOptions {
	return checker.AuthOptions{
		Mode:          a.Mode,
		AccessToken:   a.AccessToken,
		CookieEnabled: a.CookieEnabled,
		CookieFile:    a.CookieFile,
	}
}

type RecordingConfig struct {
	QualityMode                string
	ContainerMode              string
	SaveInfoText               bool
	SaveCommentsText           bool
	SaveCommentsTextFile       bool
	CommentTextTemplate        string
	MinDurationSeconds         int
	MinFileSizeMB              int
	StartupStaggerSeconds      int
	FFmpegPath                 string
	FFprobePath                string
	WorkerEnabled              bool
	WorkerPath                 string
	WorkerCheckIntervalSeconds int
	WorkerMaxRestarts          int
	ProxyEnabled               bool
	ProxyURL                   string
}

func (c RecordingConfig) proxyURL() string {
	if !c.ProxyEnabled {
		return ""
	}
	return strings.TrimSpace(c.ProxyURL)
}

type ValidationManager struct {
	activeRecordings sync.Map // map[string]bool - currently recording
	pausedStreamers  sync.Map // map[string]bool - monitoring paused
	workerHandles    sync.Map // map[string]*workerproc.Handle - isolated monitor workers
	workerRestarts   sync.Map // map[string]int - consecutive worker restarts
	workerReloads    sync.Map // map[string]bool - config reload pending until recording finishes
	restrictedLives  sync.Map // map[string]string - live session suppressed after repeated access failures
	restrictedChecks sync.Map // map[string]restrictedCheck - pending restricted access confirmation
	checkFailures    sync.Map // map[string]int - consecutive transient status check failures
	streamerSettings sync.Map // map[string]config.StreamerConfig
	cronEntries      sync.Map // map[string]cron.EntryID
	cron             *cron.Cron
	notifier         StatusNotifier

	outputDirMu sync.RWMutex
	outputDir   string

	authMu sync.RWMutex
	auth   AuthConfig

	recordingMu sync.RWMutex
	recording   RecordingConfig

	notificationsMu sync.RWMutex
	notifications   config.NotificationsConfig
}

type restrictedCheck struct {
	LiveKey string
	Count   int
}

func NewManager(notifier StatusNotifier, cfg *config.Config) *ValidationManager {
	outputDir := "."
	auth := AuthConfig{Mode: "auto", CookieFile: paths.DefaultCookiesPath()}
	recording := RecordingConfig{QualityMode: "stable", ContainerMode: "mkv", MinDurationSeconds: 10, StartupStaggerSeconds: 2, WorkerCheckIntervalSeconds: 30, WorkerMaxRestarts: 8}

	if cfg != nil {
		if cfg.OutputDirectory != "" {
			outputDir = cfg.OutputDirectory
		}
		auth.Mode = cfg.AuthMode
		auth.AccessToken = cfg.OAuth.AccessToken
		auth.CookieEnabled = cfg.Cookies.Enabled
		auth.CookieFile = cfg.Cookies.FilePath
		if auth.CookieFile == "" {
			auth.CookieFile = paths.DefaultCookiesPath()
		}
		recording.QualityMode = cfg.Recording.QualityMode
		recording.ContainerMode = cfg.Recording.ContainerMode
		recording.SaveInfoText = cfg.Recording.SaveInfoText
		recording.SaveCommentsText = cfg.Recording.SaveCommentsText
		recording.SaveCommentsTextFile = cfg.Recording.SaveCommentsTextFile
		recording.CommentTextTemplate = cfg.Recording.CommentTextTemplate
		recording.MinDurationSeconds = cfg.Recording.MinDurationSeconds
		recording.MinFileSizeMB = cfg.Recording.MinFileSizeMB
		recording.StartupStaggerSeconds = cfg.Recording.StartupStaggerSeconds
		recording.FFmpegPath = cfg.Recording.FFmpegPath
		recording.FFprobePath = cfg.Recording.FFprobePath
		recording.WorkerEnabled = cfg.Recording.WorkerEnabled
		recording.WorkerPath = cfg.Recording.WorkerPath
		recording.WorkerCheckIntervalSeconds = cfg.Recording.WorkerCheckIntervalSeconds
		recording.WorkerMaxRestarts = cfg.Recording.WorkerMaxRestarts
		recording.ProxyEnabled = cfg.Proxy.Enabled
		recording.ProxyURL = cfg.Proxy.URL
	}

	return &ValidationManager{
		cron:          cron.New(),
		notifier:      notifier,
		outputDir:     outputDir,
		auth:          auth,
		recording:     recording,
		notifications: cfgNotifications(cfg),
	}
}

func cfgNotifications(cfg *config.Config) config.NotificationsConfig {
	if cfg == nil {
		return config.NotificationsConfig{}
	}
	return cfg.Notifications
}

func (m *ValidationManager) Start() {
	m.cron.Start()
}

func (m *ValidationManager) Stop() {
	m.workerHandles.Range(func(key, value any) bool {
		if screenID, ok := key.(string); ok {
			m.pausedStreamers.Store(screenID, true)
		}
		if handle, ok := value.(*workerproc.Handle); ok {
			if err := handle.Stop(10 * time.Second); err != nil {
				fmt.Printf("Error stopping worker during shutdown: %v\n", err)
			}
		}
		m.workerHandles.Delete(key)
		return true
	})

	m.activeRecordings.Range(func(key, value any) bool {
		screenID, ok := key.(string)
		if ok {
			if err := recorder.StopRecording(screenID); err != nil {
				fmt.Printf("Error stopping recording for %s during shutdown: %v\n", screenID, err)
			}
		}
		return true
	})
	m.cron.Stop()
}

func (m *ValidationManager) SetOutputDirectory(outputDir string) {
	if outputDir == "" {
		outputDir = "."
	}
	m.outputDirMu.Lock()
	m.outputDir = outputDir
	m.outputDirMu.Unlock()
}

func (m *ValidationManager) SetAuthConfig(auth AuthConfig) {
	if auth.Mode == "" {
		auth.Mode = "auto"
	}
	if auth.CookieFile == "" {
		auth.CookieFile = paths.DefaultCookiesPath()
	}
	m.authMu.Lock()
	changed := m.auth != auth
	m.auth = auth
	m.authMu.Unlock()
	if !changed {
		return
	}
	m.restrictedLives.Range(func(key, _ any) bool {
		m.restrictedLives.Delete(key)
		return true
	})
	m.restrictedChecks.Range(func(key, _ any) bool {
		m.restrictedChecks.Delete(key)
		return true
	})
}

func (m *ValidationManager) SetRecordingConfig(recording RecordingConfig) {
	if strings.TrimSpace(recording.QualityMode) == "" {
		recording.QualityMode = "stable"
	}
	if strings.TrimSpace(recording.ContainerMode) == "" {
		recording.ContainerMode = "mkv"
	}
	if recording.StartupStaggerSeconds < 0 {
		recording.StartupStaggerSeconds = 0
	}
	if recording.WorkerCheckIntervalSeconds <= 0 {
		recording.WorkerCheckIntervalSeconds = 30
	}
	if recording.WorkerCheckIntervalSeconds < 5 {
		recording.WorkerCheckIntervalSeconds = 5
	}
	if recording.WorkerCheckIntervalSeconds > 300 {
		recording.WorkerCheckIntervalSeconds = 300
	}
	if recording.WorkerMaxRestarts <= 0 {
		recording.WorkerMaxRestarts = 8
	}
	if recording.WorkerMaxRestarts > 100 {
		recording.WorkerMaxRestarts = 100
	}
	if recording.MinDurationSeconds <= 0 {
		recording.MinDurationSeconds = 10
	}
	if recording.MinDurationSeconds > 3600 {
		recording.MinDurationSeconds = 3600
	}
	if recording.MinFileSizeMB < 0 {
		recording.MinFileSizeMB = 0
	}
	if recording.MinFileSizeMB > 1024 {
		recording.MinFileSizeMB = 1024
	}
	recording.CommentTextTemplate = strings.TrimSpace(recording.CommentTextTemplate)
	if recording.CommentTextTemplate == "" {
		recording.CommentTextTemplate = "[{offset}] {display_name}: {message}"
	}
	recording.ProxyURL = strings.TrimSpace(recording.ProxyURL)
	if recording.ProxyURL == "" {
		recording.ProxyEnabled = false
	}
	m.recordingMu.Lock()
	m.recording = recording
	m.recordingMu.Unlock()
}

func (m *ValidationManager) SetNotificationsConfig(notifications config.NotificationsConfig) {
	notifications.Telegram.BotToken = strings.TrimSpace(notifications.Telegram.BotToken)
	notifications.Telegram.ChatID = strings.TrimSpace(notifications.Telegram.ChatID)
	if notifications.Telegram.BotToken == "" || notifications.Telegram.ChatID == "" {
		notifications.Telegram.Enabled = false
	}
	m.notificationsMu.Lock()
	m.notifications = notifications
	m.notificationsMu.Unlock()
}

// ReloadIdleWorkers applies updated settings to isolated workers that are not
// currently recording. Active recordings keep their launch-time settings and
// pick up the new configuration on the next live.
func (m *ValidationManager) ReloadIdleWorkers() {
	workerEnabled := m.getRecordingConfig().WorkerEnabled
	m.workerHandles.Range(func(key, _ any) bool {
		screenID, ok := key.(string)
		if !ok {
			return true
		}
		if _, recording := m.activeRecordings.Load(screenID); recording {
			m.scheduleWorkerReload(screenID)
			return true
		}
		go func() {
			m.stopWorkerMonitor(screenID)
			if workerEnabled && m.IsMonitoring(screenID) {
				m.startWorkerMonitor(screenID)
			}
		}()
		return true
	})
	if workerEnabled {
		m.streamerSettings.Range(func(key, _ any) bool {
			screenID, ok := key.(string)
			if !ok || !m.IsMonitoring(screenID) {
				return true
			}
			if _, recording := m.activeRecordings.Load(screenID); recording {
				return true
			}
			if _, exists := m.workerHandles.Load(screenID); exists {
				return true
			}
			go m.startWorkerMonitor(screenID)
			return true
		})
	}
}

func (m *ValidationManager) scheduleWorkerReload(screenID string) {
	if _, loaded := m.workerReloads.LoadOrStore(screenID, true); loaded {
		return
	}
	go func() {
		defer m.workerReloads.Delete(screenID)
		for {
			if _, exists := m.getStreamerConfig(screenID); !exists {
				return
			}
			if !m.IsMonitoring(screenID) {
				return
			}
			if _, recording := m.activeRecordings.Load(screenID); !recording {
				break
			}
			time.Sleep(time.Second)
		}
		m.stopWorkerMonitor(screenID)
		if _, exists := m.getStreamerConfig(screenID); exists && m.getRecordingConfig().WorkerEnabled && m.IsMonitoring(screenID) {
			m.startWorkerMonitor(screenID)
		}
	}()
}

func (m *ValidationManager) getOutputDirectory() string {
	m.outputDirMu.RLock()
	defer m.outputDirMu.RUnlock()
	if m.outputDir == "" {
		return "."
	}
	return m.outputDir
}

func (m *ValidationManager) getAuthConfig() AuthConfig {
	m.authMu.RLock()
	defer m.authMu.RUnlock()
	return m.auth
}

func (m *ValidationManager) getAuthConfigForStreamer(screenID string) AuthConfig {
	auth := m.getAuthConfig()
	if auth.Mode == "" {
		auth.Mode = "auto"
	}
	if auth.CookieFile == "" {
		auth.CookieFile = paths.DefaultCookiesPath()
	}
	if value, ok := m.streamerSettings.Load(screenID); ok {
		if streamer, okCast := value.(config.StreamerConfig); okCast {
			switch strings.ToLower(strings.TrimSpace(streamer.AuthMode)) {
			case "cookie":
				auth.Mode = "cookie"
				auth.CookieEnabled = true
			case "no_cookie":
				auth.Mode = "oauth"
				auth.CookieEnabled = false
			}
		}
	}
	return auth
}

func (m *ValidationManager) getRecordingConfig() RecordingConfig {
	m.recordingMu.RLock()
	defer m.recordingMu.RUnlock()
	return m.recording
}

func (m *ValidationManager) getNotificationsConfig() config.NotificationsConfig {
	m.notificationsMu.RLock()
	defer m.notificationsMu.RUnlock()
	return m.notifications
}

func (m *ValidationManager) getRecordingConfigForStreamer(screenID string) RecordingConfig {
	recording := m.getRecordingConfig()
	if value, ok := m.streamerSettings.Load(screenID); ok {
		if streamer, okCast := value.(config.StreamerConfig); okCast {
			if mode := strings.TrimSpace(streamer.QualityMode); mode != "" {
				recording.QualityMode = mode
			}
			if mode := strings.TrimSpace(streamer.ContainerMode); mode != "" {
				recording.ContainerMode = mode
			}
		}
	}
	return recording
}

func (m *ValidationManager) ToggleMonitoring(screenID string) bool {
	if _, recording := m.activeRecordings.Load(screenID); recording {
		m.SetMonitoring(screenID, false)
		return false
	}
	if m.IsMonitoring(screenID) {
		m.SetMonitoring(screenID, false)
		return false
	}
	m.SetMonitoring(screenID, true)
	return true
}

func (m *ValidationManager) SetMonitoring(screenID string, monitoring bool) {
	if monitoring {
		m.pausedStreamers.Delete(screenID)
		m.workerRestarts.Delete(screenID)
		if m.notifier != nil {
			m.notifier.NotifyStatus(screenID, "monitoring", "Monitoring started")
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Monitoring started", screenID))
		}
		if m.getRecordingConfig().WorkerEnabled {
			m.startWorkerMonitor(screenID)
			return
		}
		go m.checkAndRecordImmediate(screenID)
		return
	}

	// Mark paused first to avoid race with cron checker.
	m.pausedStreamers.Store(screenID, true)
	if m.notifier != nil {
		m.notifier.NotifyStatus(screenID, "idle", "Monitoring paused")
		m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Pause requested", screenID))
	}

	if err := recorder.StopRecording(screenID); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no active recording") {
			fmt.Printf("Error stopping recording for %s: %v\n", screenID, err)
			if m.notifier != nil {
				m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Stop recording failed: %v", screenID, err))
			}
		}
	}
	m.stopWorkerMonitor(screenID)
}

func (m *ValidationManager) IsMonitoring(screenID string) bool {
	if _, paused := m.pausedStreamers.Load(screenID); paused {
		return false
	}
	return true
}

func (m *ValidationManager) AddStreamer(streamer config.StreamerConfig) error {
	if streamer.ScreenID == "" {
		return fmt.Errorf("screen_id is empty")
	}
	m.streamerSettings.Store(streamer.ScreenID, streamer)

	if existingID, ok := m.cronEntries.Load(streamer.ScreenID); ok {
		if entryID, okCast := existingID.(cron.EntryID); okCast {
			m.cron.Remove(entryID)
		}
		m.cronEntries.Delete(streamer.ScreenID)
	}

	screenID := streamer.ScreenID
	entryID, err := m.cron.AddFunc(streamer.Schedule, func() {
		m.checkAndRecord(screenID)
	})
	if err != nil {
		return err
	}

	m.cronEntries.Store(streamer.ScreenID, entryID)
	return nil
}

func (m *ValidationManager) RemoveStreamer(screenID string) error {
	if existingID, ok := m.cronEntries.Load(screenID); ok {
		if entryID, okCast := existingID.(cron.EntryID); okCast {
			m.cron.Remove(entryID)
		}
		m.cronEntries.Delete(screenID)
	}

	if _, recording := m.activeRecordings.Load(screenID); recording {
		if err := recorder.StopRecording(screenID); err != nil {
			fmt.Printf("Error stopping recording for %s during remove: %v\n", screenID, err)
		}
	}
	m.stopWorkerMonitor(screenID)

	m.activeRecordings.Delete(screenID)
	m.pausedStreamers.Delete(screenID)
	m.workerRestarts.Delete(screenID)
	m.workerReloads.Delete(screenID)
	m.restrictedLives.Delete(screenID)
	m.restrictedChecks.Delete(screenID)
	m.checkFailures.Delete(screenID)
	m.streamerSettings.Delete(screenID)
	return nil
}

func (m *ValidationManager) checkAndRecord(screenID string) {
	m.checkAndRecordWithOptions(screenID, false)
}

func (m *ValidationManager) checkAndRecordImmediate(screenID string) {
	m.checkAndRecordWithOptions(screenID, true)
}

func (m *ValidationManager) checkAndRecordWithOptions(screenID string, immediate bool) {
	if _, paused := m.pausedStreamers.Load(screenID); paused {
		return
	}

	recordingConfig := m.getRecordingConfigForStreamer(screenID)
	checker.SetProxyURL(recordingConfig.proxyURL())
	if recordingConfig.WorkerEnabled {
		m.startWorkerMonitor(screenID)
		return
	}

	if _, recording := m.activeRecordings.Load(screenID); recording {
		return
	}

	fmt.Printf("Checking status for %s...\n", screenID)
	auth := m.getAuthConfigForStreamer(screenID)
	if !immediate && isForcedCookieAuth(auth) && !m.waitForForcedCookieCheckSlot(screenID) {
		return
	}
	info, err := checker.CheckStreamStatusWithAuth(screenID, auth.checkerOptions())
	if err != nil {
		fmt.Printf("Error checking %s: %v\n", screenID, err)
		if m.notifier != nil {
			if protected, ok := checker.AsProtectedLiveError(err); ok {
				m.clearCheckFailure(screenID)
				if m.suppressRestrictedLive(screenID, protected.LiveKey) {
					m.notifier.NotifyStatus(screenID, "restricted", "受限直播：当前账号无观看权限")
					m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Protected live detected; current account has no viewing permission. Further attempts are suppressed until this live ends", screenID))
				}
				return
			}
			loggedFailure := false
			if isForcedCookieAuth(auth) && isTransientStatusCheckError(err) {
				failures := m.recordCheckFailure(screenID)
				m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Check live status failed: %v", screenID, err))
				loggedFailure = true
				if failures < forcedCookieCheckFailureThreshold {
					m.notifier.NotifyStatus(screenID, "monitoring", fmt.Sprintf("状态检查暂时超时，等待重试 (%d/%d)", failures, forcedCookieCheckFailureThreshold))
					return
				}
			}
			m.notifier.NotifyStatus(screenID, "error", fmt.Sprintf("Error: %v", err))
			if !loggedFailure {
				m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Check live status failed: %v", screenID, err))
			}
		}
		return
	}
	m.clearCheckFailure(screenID)

	if info.IsLive {
		liveKey := checker.LiveSessionKey(info)
		if isForcedCookieAuth(auth) && m.isRestrictedLive(screenID, liveKey) {
			return
		}
		if m.notifier != nil && auth.CookieEnabled && (auth.Mode == "auto" || auth.Mode == "cookie") {
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Cookie auth used for live URL resolution", screenID))
		}
		confirmedInfo, rejectReason := m.confirmLiveBeforeRecording(screenID, info, auth)
		if rejectReason != "" {
			if m.notifier != nil {
				m.notifier.NotifyStatus(screenID, "monitoring", "Live check was not confirmed")
				m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Live check ignored: %s", screenID, rejectReason))
			}
			return
		}
		info = confirmedInfo
		liveKey = checker.LiveSessionKey(info)
		if isForcedCookieAuth(auth) && m.isRestrictedLive(screenID, liveKey) {
			return
		}

		fmt.Printf("%s is LIVE! Title: %s. Stream URL resolved. Starting recording...\n", screenID, info.Title)
		if m.notifier != nil {
			m.notifier.NotifyStatus(screenID, "recording", fmt.Sprintf("Live! %s", info.Title))
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Live started, recording: %s", screenID, info.Title))
		}
		m.activeRecordings.Store(screenID, true)
		m.sendRecordingNotification("start", screenID, m.recordingStartMessage(screenID, info.Title))

		streamTitle := info.Title
		streamURL := info.StreamURL
		outputDir := m.getOutputDirectory()
		go func(sID, sTitle, sURL, outDir, sessionKey string, ac AuthConfig, rc RecordingConfig) {
			recordingFailed := false
			defer m.activeRecordings.Delete(sID)
			defer func() {
				if m.notifier != nil {
					if _, paused := m.pausedStreamers.Load(sID); !paused {
						if recordingFailed {
							return
						}
						m.notifier.NotifyStatus(sID, "monitoring", "Recording finished, waiting for next stream")
					}
				}
			}()

			recAuth := recorder.AuthSettings{
				Mode:          ac.Mode,
				AccessToken:   ac.AccessToken,
				CookieEnabled: ac.CookieEnabled,
				CookieFile:    ac.CookieFile,
			}
			recordOptions := recorder.RecordOptions{
				QualityMode:          rc.QualityMode,
				ContainerMode:        rc.ContainerMode,
				SaveInfoText:         rc.SaveInfoText,
				SaveCommentsText:     rc.SaveCommentsText,
				SaveCommentsTextFile: rc.SaveCommentsTextFile,
				CommentTextTemplate:  rc.CommentTextTemplate,
				MovieID:              info.MovieID,
				FFmpegPath:           rc.FFmpegPath,
				FFprobePath:          rc.FFprobePath,
				ProxyURL:             rc.proxyURL(),
			}
			var duration, filePath string
			var fileSize int64
			var stoppedByUser bool
			var recErr error
			duration, filePath, fileSize, stoppedByUser, recErr = recorder.RecordLiveStreamWithOptions(sID, sTitle, sURL, outDir, recAuth, recordOptions, m.notifier)

			if filePath != "" && m.notifier != nil && (fileSize > 0 || stoppedByUser) {
				historyStatus := classifyRecordingStatus(duration, fileSize, stoppedByUser, recErr, rc)
				m.notifier.AddRecordingHistoryWithStatus(sID, filePath, duration, fileSize, historyStatus)
			}

			if recErr != nil {
				fmt.Printf("Recording error for %s: %v\n", sID, recErr)
				if !stoppedByUser && isForcedCookieAuth(ac) && recorder.IsRestrictedAccessError(recErr) {
					recordingFailed = true
					attempt := m.recordRestrictedFailure(sID, sessionKey)
					if m.notifier != nil {
						if attempt >= 2 {
							m.notifier.NotifyStatus(sID, "restricted", "受限直播：当前账号无观看权限")
							m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Restricted live detected; current Cookie account has no viewing permission. Further attempts are suppressed until this live ends", sID))
						} else {
							m.notifier.NotifyStatus(sID, "monitoring", "受限直播验证失败，等待再次确认...")
							m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Restricted live access check failed once; waiting for one confirmation retry", sID))
						}
					}
					return
				}
				m.restrictedChecks.Delete(sID)
				if m.notifier != nil && !stoppedByUser && recErr.Error() != "signal: killed" {
					recordingFailed = true
					m.notifier.NotifyStatus(sID, "error", fmt.Sprintf("Recording failed: %v", recErr))
					m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Recording failed: %v", sID, recErr))
					m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Automatic immediate retry skipped; waiting for next scheduled check", sID))
					m.sendRecordingNotification("error", sID, m.recordingErrorMessage(sID, sTitle, recErr.Error()))
				}
			} else if !stoppedByUser {
				m.restrictedChecks.Delete(sID)
				m.sendRecordingNotification("finish", sID, m.recordingFinishMessage(sID, sTitle, duration, filePath, fileSize))
			}
		}(screenID, streamTitle, streamURL, outputDir, liveKey, auth, recordingConfig)
	} else {
		if m.clearRestrictedLive(screenID) && m.notifier != nil {
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Restricted live ended; monitoring resumed", screenID))
		}
		fmt.Printf("%s is offline.\n", screenID)
		if m.notifier != nil {
			m.notifier.NotifyStatus(screenID, "monitoring", "Monitoring for live stream...")
		}
	}
}

func (m *ValidationManager) confirmLiveBeforeRecording(screenID string, first *checker.StreamInfo, auth AuthConfig) (*checker.StreamInfo, string) {
	if first == nil || !first.IsLive {
		return nil, "first check is not live"
	}
	if !checker.IsRecordableLiveURL(first.StreamURL) {
		return nil, "first check returned a non-recordable stream URL"
	}
	if m.notifier != nil {
		m.notifier.NotifyStatus(screenID, "monitoring", "Live candidate detected, confirming...")
	}

	time.Sleep(liveConfirmDelay)

	if _, paused := m.pausedStreamers.Load(screenID); paused {
		return nil, "monitoring paused during live confirmation"
	}
	if _, recording := m.activeRecordings.Load(screenID); recording {
		return nil, "recording already active during live confirmation"
	}

	second, err := checker.CheckStreamStatusWithAuth(screenID, auth.checkerOptions())
	if err != nil {
		return nil, fmt.Sprintf("second check failed: %v", err)
	}
	if second == nil || !second.IsLive {
		return nil, "second check is not live"
	}
	if !checker.IsRecordableLiveURL(second.StreamURL) {
		return nil, "second check returned a non-recordable stream URL"
	}
	return second, ""
}

func (m *ValidationManager) sendRecordingNotification(kind, screenID, message string) {
	if !m.telegramEnabledForStreamer(screenID) {
		return
	}
	notifications := m.getNotificationsConfig()
	tg := notifications.Telegram
	if !tg.Enabled {
		return
	}
	switch kind {
	case "start":
		if !tg.NotifyOnStart {
			return
		}
	case "finish":
		if !tg.NotifyOnFinish {
			return
		}
	case "error":
		if !tg.NotifyOnError {
			return
		}
	default:
		return
	}
	proxyURL := m.getRecordingConfigForStreamer(screenID).proxyURL()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := notify.SendTelegram(ctx, tg, proxyURL, message); err != nil && m.notifier != nil {
			m.notifier.NotifyAppLog(fmt.Sprintf("[%s] Telegram push failed: %v", screenID, err))
		}
	}()
}
