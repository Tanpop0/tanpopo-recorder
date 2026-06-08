package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/auth"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/history"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/metadata"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/notify"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/recorder"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/scheduler"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx            context.Context
	config         *config.Config
	scheduler      *scheduler.ValidationManager
	historyManager *history.HistoryManager
	isQuitting     atomic.Bool
	bulkOperation  atomic.Uint64
	shutdownOnce   sync.Once
	mu             sync.RWMutex
	diagMu         sync.RWMutex
	diagnostics    map[string]*StreamerDiagnostics
	statusMu       sync.RWMutex
	runtimeStatus  map[string]StreamerRuntimeStatus
}

type SettingsPayload struct {
	OutputDirectory string                     `json:"output_directory"`
	AuthMode        string                     `json:"auth_mode"`
	OAuth           config.OAuthConfig         `json:"oauth"`
	Cookies         config.CookieConfig        `json:"cookies"`
	Recording       config.RecordingConfig     `json:"recording"`
	Proxy           config.ProxyConfig         `json:"proxy"`
	Notifications   config.NotificationsConfig `json:"notifications"`
}

type StreamerDiagnostics struct {
	ScreenID     string   `json:"screen_id"`
	LastError    string   `json:"last_error"`
	LastFilePath string   `json:"last_file_path"`
	RecentLogs   []string `json:"recent_logs"`
}

type StreamerRuntimeStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type HealthCheckItem struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type HealthCheckReport struct {
	OK    bool              `json:"ok"`
	Items []HealthCheckItem `json:"items"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		diagnostics:   make(map[string]*StreamerDiagnostics),
		runtimeStatus: make(map[string]StreamerRuntimeStatus),
	}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Printf("Error loading config: %v. Creating default.", err)
		cfg = &config.Config{
			Streamers:       []config.StreamerConfig{},
			OutputDirectory: ".",
			AuthMode:        "auto",
			Cookies:         config.CookieConfig{Enabled: false, FilePath: "cookies.txt"},
			Recording:       config.RecordingConfig{QualityMode: "stable", ContainerMode: "mkv", SaveCommentsText: true, CommentTextTemplate: "[{offset}] {display_name}: {message}", StartupStaggerSeconds: 2, WorkerCheckIntervalSeconds: 30, WorkerMaxRestarts: 8},
			Notifications:   config.NotificationsConfig{Telegram: config.TelegramConfig{NotifyOnStart: true, NotifyOnFinish: true, NotifyOnError: true}},
		}
		if errCreate := config.SaveConfig("config.yaml", cfg); errCreate != nil {
			log.Printf("Failed to create default config: %v", errCreate)
		}
	}

	a.mu.Lock()
	a.config = cfg
	a.mu.Unlock()
	proxyURL := ""
	if cfg.Proxy.Enabled {
		proxyURL = cfg.Proxy.URL
	}
	auth.SetProxyURL(proxyURL)
	metadata.SetProxyURL(proxyURL)

	a.scheduler = scheduler.NewManager(a, cfg)
	a.historyManager = history.NewHistoryManager("history.json")

	for _, s := range cfg.Streamers {
		if err := a.scheduler.AddStreamer(s); err != nil {
			log.Printf("Failed to schedule %s: %v", s.ScreenID, err)
		} else {
			a.scheduler.SetMonitoring(s.ScreenID, false)
		}
	}

	a.scheduler.Start()
	log.Println("Scheduler started")
}

func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	if a.isQuitting.Load() {
		return false
	}
	a.NotifyAppLog("窗口已隐藏到后台；录制和监听继续运行")
	a.HideWindow()
	return true
}

func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	runtime.Show(a.ctx)
	runtime.WindowShow(a.ctx)
	if stdruntime.GOOS == "windows" {
		runtime.WindowUnminimise(a.ctx)
	}
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	go func(ctx context.Context) {
		time.Sleep(250 * time.Millisecond)
		runtime.WindowSetAlwaysOnTop(ctx, false)
	}(a.ctx)
}

func (a *App) HideWindow() {
	if a.ctx != nil {
		runtime.WindowHide(a.ctx)
	}
}

func (a *App) Quit() {
	a.isQuitting.Store(true)
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
	go func() {
		time.Sleep(12 * time.Second)
		a.ForceQuit()
	}()
}

func (a *App) ForceQuit() {
	a.isQuitting.Store(true)
	a.stopBackground()
	os.Exit(0)
}

func (a *App) AddRecordingHistory(streamerID, filePath, duration string, fileSize int64) {
	a.AddRecordingHistoryWithStatus(streamerID, filePath, duration, fileSize, "completed")
}

func (a *App) AddRecordingHistoryWithStatus(streamerID, filePath, duration string, fileSize int64, status string) {
	if a.historyManager == nil {
		return
	}
	if strings.TrimSpace(status) == "" {
		status = "completed"
	}

	now := time.Now()
	record := history.RecordingRecord{
		ID:         uuid.New().String(),
		StreamerID: streamerID,
		FilePath:   filePath,
		FileSize:   fileSize,
		Duration:   duration,
		StartTime:  now,
		EndTime:    now,
		Status:     status,
	}

	if err := a.historyManager.AddRecord(record); err != nil {
		log.Printf("Failed to add history record: %v", err)
		return
	}
	a.rememberLastFile(streamerID, filePath)

	runtime.EventsEmit(a.ctx, "history-updated", map[string]string{
		"streamer_id": streamerID,
		"status":      status,
	})
}

func (a *App) GetRecordingHistory() []history.RecordingRecord {
	if a.historyManager == nil {
		return []history.RecordingRecord{}
	}
	records := a.historyManager.GetRecords()
	streamers := a.streamerMetaSnapshot()
	for i := range records {
		fillHistorySidecars(&records[i])
		if meta, ok := streamers[normalizeID(records[i].StreamerID)]; ok {
			records[i].Nickname = meta.Nickname
			records[i].Avatar = meta.Avatar
		}
	}
	return records
}

type historyStreamerMeta struct {
	Nickname string
	Avatar   string
}

func (a *App) streamerMetaSnapshot() map[string]historyStreamerMeta {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.config == nil {
		return nil
	}
	out := make(map[string]historyStreamerMeta, len(a.config.Streamers))
	for _, streamer := range a.config.Streamers {
		screenID := normalizeID(streamer.ScreenID)
		if screenID == "" {
			continue
		}
		out[screenID] = historyStreamerMeta{
			Nickname: streamer.Nickname,
			Avatar:   streamer.Avatar,
		}
	}
	return out
}

func (a *App) ClearRecordingHistory() {
	if a.historyManager != nil {
		if err := a.historyManager.ClearHistory(); err != nil {
			log.Printf("Failed to clear history: %v", err)
			return
		}
	}
	runtime.EventsEmit(a.ctx, "history-updated", map[string]string{
		"action": "clear",
	})
}

func (a *App) DeleteHistoryRecord(id string) {
	if a.historyManager != nil {
		if err := a.historyManager.DeleteRecord(id); err != nil {
			log.Printf("Failed to delete history record %s: %v", id, err)
			return
		}
	}
	runtime.EventsEmit(a.ctx, "history-updated", map[string]string{
		"action": "delete",
		"id":     id,
	})
}

func (a *App) DeleteHistoryRecordAndFile(id string) string {
	if a.historyManager == nil {
		return "history manager not initialized"
	}

	record, ok := a.historyManager.GetRecord(id)
	if !ok {
		return "history record not found"
	}

	filePath := strings.TrimSpace(record.FilePath)
	if filePath == "" {
		return "record has no file path"
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Sprintf("stat file failed: %v", err)
		}
	} else {
		if info.IsDir() {
			return "refuse to delete a directory"
		}
		if err := os.Remove(filePath); err != nil {
			return fmt.Sprintf("delete file failed: %v", err)
		}
	}
	for _, sidecar := range recordingSidecarPaths(filePath) {
		if info, err := os.Stat(sidecar); err == nil && !info.IsDir() {
			_ = os.Remove(sidecar)
		}
	}

	if err := a.historyManager.DeleteRecord(id); err != nil {
		return fmt.Sprintf("delete history record failed: %v", err)
	}

	runtime.EventsEmit(a.ctx, "history-updated", map[string]string{
		"action": "delete_file",
		"id":     id,
	})
	return ""
}

func recordingSidecarPaths(filePath string) []string {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return nil
	}
	base := strings.TrimSuffix(filePath, filepath.Ext(filePath))
	return []string{
		base + ".txt",
		base + ".comments.txt",
		base + ".comments.jsonl",
	}
}

func fillHistorySidecars(record *history.RecordingRecord) {
	if record == nil {
		return
	}
	filePath := strings.TrimSpace(record.FilePath)
	if filePath == "" {
		return
	}
	base := strings.TrimSuffix(filePath, filepath.Ext(filePath))
	record.CommentTextPath = base + ".comments.txt"
	record.CommentJSONLPath = base + ".comments.jsonl"
	record.CommentTextExists = regularFileExists(record.CommentTextPath)
	record.CommentJSONLExists = regularFileExists(record.CommentJSONLPath)
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (a *App) UpdateHistoryRecordStatus(id string, status string) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "completed"
	}
	if a.historyManager != nil {
		if err := a.historyManager.UpdateRecordStatus(id, status); err != nil {
			log.Printf("Failed to update history record %s: %v", id, err)
			return
		}
	}
	runtime.EventsEmit(a.ctx, "history-updated", map[string]string{
		"action": "status",
		"id":     id,
		"status": status,
	})
}

func (a *App) NotifyStatus(screenID, status, message string) {
	screenID = normalizeID(screenID)
	a.statusMu.Lock()
	a.runtimeStatus[screenID] = StreamerRuntimeStatus{
		Status:  strings.TrimSpace(status),
		Message: strings.TrimSpace(message),
	}
	a.statusMu.Unlock()

	runtime.EventsEmit(a.ctx, "streamer-status", map[string]string{
		"screen_id": screenID,
		"status":    status,
		"message":   message,
	})
}

func (a *App) NotifyAppLog(message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	a.rememberStreamerLog(message)
	runtime.EventsEmit(a.ctx, "app-log", map[string]string{
		"message": message,
	})
}

func (a *App) rememberLastFile(screenID, filePath string) {
	screenID = normalizeID(screenID)
	if screenID == "" || strings.TrimSpace(filePath) == "" {
		return
	}
	a.diagMu.Lock()
	diag := a.ensureDiagnosticsLocked(screenID)
	diag.LastFilePath = filePath
	snapshot := cloneDiagnostics(diag)
	a.diagMu.Unlock()
	a.emitDiagnostics(snapshot)
}

func (a *App) rememberStreamerLog(message string) {
	screenID, line := parseStreamerLogLine(message)
	if screenID == "" || line == "" {
		return
	}
	a.appendStreamerLogFile(screenID, line)
	displayLine := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), line)

	a.diagMu.Lock()
	diag := a.ensureDiagnosticsLocked(screenID)
	diag.RecentLogs = append([]string{displayLine}, diag.RecentLogs...)
	if len(diag.RecentLogs) > 20 {
		diag.RecentLogs = diag.RecentLogs[:20]
	}
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "exited") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "unauthorized") {
		diag.LastError = displayLine
	}
	snapshot := cloneDiagnostics(diag)
	a.diagMu.Unlock()
	a.emitDiagnostics(snapshot)
}

func (a *App) appendStreamerLogFile(screenID, line string) {
	logDir := "logs"
	a.mu.RLock()
	if a.config != nil && strings.TrimSpace(a.config.OutputDirectory) != "" {
		logDir = filepath.Join(a.config.OutputDirectory, "logs")
	}
	a.mu.RUnlock()

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return
	}
	logPath := filepath.Join(logDir, sanitizeLogName(screenID)+".log")
	rotateLogFile(logPath, 2*1024*1024)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\r\n", time.Now().Format(time.RFC3339), line)
}

func sanitizeLogName(s string) string {
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	s = strings.TrimSpace(replacer.Replace(s))
	if s == "" {
		return "unknown"
	}
	return s
}

func rotateLogFile(path string, maxBytes int64) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxBytes {
		return
	}
	backup := strings.TrimSuffix(path, filepath.Ext(path)) + ".1" + filepath.Ext(path)
	_ = os.Remove(backup)
	_ = os.Rename(path, backup)
}

func (a *App) ensureDiagnosticsLocked(screenID string) *StreamerDiagnostics {
	if a.diagnostics == nil {
		a.diagnostics = make(map[string]*StreamerDiagnostics)
	}
	diag := a.diagnostics[screenID]
	if diag == nil {
		diag = &StreamerDiagnostics{ScreenID: screenID}
		a.diagnostics[screenID] = diag
	}
	return diag
}

func cloneDiagnostics(diag *StreamerDiagnostics) StreamerDiagnostics {
	if diag == nil {
		return StreamerDiagnostics{}
	}
	return StreamerDiagnostics{
		ScreenID:     diag.ScreenID,
		LastError:    diag.LastError,
		LastFilePath: diag.LastFilePath,
		RecentLogs:   append([]string(nil), diag.RecentLogs...),
	}
}

func parseStreamerLogLine(message string) (string, string) {
	message = strings.TrimSpace(message)
	if !strings.HasPrefix(message, "[") {
		return "", ""
	}
	end := strings.Index(message, "]")
	if end <= 1 {
		return "", ""
	}
	screenID := strings.TrimSpace(message[1:end])
	if screenID == "" || strings.ContainsAny(screenID, " \t\r\n") {
		return "", ""
	}
	return screenID, message
}

func (a *App) emitDiagnostics(diag StreamerDiagnostics) {
	if a.ctx == nil || diag.ScreenID == "" {
		return
	}
	runtime.EventsEmit(a.ctx, "streamer-diagnostics", diag)
}

func (a *App) GetStreamerDiagnostics(screenID string) StreamerDiagnostics {
	screenID = normalizeID(screenID)
	a.diagMu.RLock()
	defer a.diagMu.RUnlock()
	return cloneDiagnostics(a.diagnostics[screenID])
}

func (a *App) CheckRecordingTools() recorder.ToolStatus {
	a.mu.RLock()
	var opts recorder.RecordOptions
	if a.config != nil {
		opts.FFmpegPath = a.config.Recording.FFmpegPath
		opts.FFprobePath = a.config.Recording.FFprobePath
	}
	a.mu.RUnlock()
	return recorder.CheckToolchain(opts)
}

func (a *App) CheckRecordingToolsWithPaths(ffmpegPath, ffprobePath string) recorder.ToolStatus {
	return recorder.CheckToolchain(recorder.RecordOptions{
		FFmpegPath:  strings.TrimSpace(ffmpegPath),
		FFprobePath: strings.TrimSpace(ffprobePath),
	})
}

func (a *App) RunHealthCheck() HealthCheckReport {
	a.mu.RLock()
	cfg := a.config
	a.mu.RUnlock()
	return a.runHealthCheckForConfig(cfg)
}

func (a *App) runHealthCheckForConfig(cfg *config.Config) HealthCheckReport {
	report := HealthCheckReport{OK: true, Items: make([]HealthCheckItem, 0, 8)}
	add := func(name, status, message string) {
		if status == "error" {
			report.OK = false
		}
		report.Items = append(report.Items, HealthCheckItem{Name: name, Status: status, Message: message})
	}

	if cfg == nil {
		add("配置", "error", "配置尚未加载")
		return report
	}

	toolStatus := recorder.CheckToolchain(recorder.RecordOptions{
		FFmpegPath:  cfg.Recording.FFmpegPath,
		FFprobePath: cfg.Recording.FFprobePath,
	})
	if toolStatus.FFmpegOK {
		add("FFmpeg", "ok", toolStatus.FFmpegPath)
	} else {
		add("FFmpeg", "error", toolStatus.Message)
	}
	if toolStatus.FFprobeOK {
		add("FFprobe", "ok", toolStatus.FFprobePath)
	} else {
		add("FFprobe", "warn", "FFprobe is not available; recording can continue, but duration probing may be less accurate")
	}

	outputDir := cfg.OutputDirectory
	if strings.TrimSpace(outputDir) == "" {
		outputDir = "."
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		add("输出目录", "error", err.Error())
	} else {
		testPath := filepath.Join(outputDir, ".twitcasting-write-test")
		if err := os.WriteFile(testPath, []byte("ok"), 0644); err != nil {
			add("输出目录", "error", "不可写: "+err.Error())
		} else {
			_ = os.Remove(testPath)
			add("输出目录", "ok", outputDir)
		}
	}

	if cfg.Recording.WorkerEnabled {
		if path, err := findWorkerExecutable(cfg.Recording.WorkerPath); err != nil {
			add("Worker", "error", err.Error())
		} else {
			add("Worker", "ok", path)
		}
	} else {
		add("Worker", "warn", "未启用 worker 进程隔离")
	}

	if cfg.Cookies.Enabled {
		if _, err := os.Stat(cfg.Cookies.FilePath); err != nil {
			add("Cookie", "warn", "已启用但文件不可读: "+err.Error())
		} else {
			add("Cookie", "ok", cfg.Cookies.FilePath)
		}
	} else {
		add("Cookie", "warn", "未启用 cookies.txt，会员或受限直播可能失败")
	}

	if strings.TrimSpace(cfg.OAuth.AccessToken) != "" {
		add("OAuth", "ok", "Access Token configured")
	} else {
		add("OAuth", "warn", "Access Token is empty; official API checks and comment capture may be limited")
	}

	if cfg.Proxy.Enabled {
		u, err := url.Parse(cfg.Proxy.URL)
		if err != nil {
			add("代理", "error", "代理地址格式不正确: "+err.Error())
		} else if u.Scheme == "" || u.Host == "" {
			add("代理", "error", "代理地址需要包含协议和主机，例如 http://127.0.0.1:7890")
		} else {
			add("代理", "ok", cfg.Proxy.URL)
		}
	} else {
		add("Proxy", "ok", "Proxy disabled")
	}

	return report
}

func (a *App) RunHealthCheckWithSettings(payload SettingsPayload) HealthCheckReport {
	a.mu.RLock()
	original := a.config
	a.mu.RUnlock()

	cfg := &config.Config{
		OutputDirectory: payload.OutputDirectory,
		AuthMode:        payload.AuthMode,
		OAuth:           payload.OAuth,
		Cookies:         payload.Cookies,
		Recording:       payload.Recording,
		Proxy:           payload.Proxy,
		Notifications:   payload.Notifications,
	}
	if original != nil {
		cfg.Streamers = append([]config.StreamerConfig(nil), original.Streamers...)
	}
	return a.runHealthCheckForConfig(cfg)
}

func findWorkerExecutable(configuredPath string) (string, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath != "" {
		if _, err := os.Stat(configuredPath); err == nil {
			return configuredPath, nil
		}
		return "", fmt.Errorf("worker executable not found: %s", configuredPath)
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

func (a *App) shutdown(ctx context.Context) {
	a.stopBackground()
}

func (a *App) stopBackground() {
	a.shutdownOnce.Do(func() {
		if a.scheduler != nil {
			a.scheduler.Stop()
		}
	})
}

// StreamerStatus represents the current state of a streamer for the GUI.
type StreamerStatus struct {
	ScreenID        string   `json:"screen_id"`
	Schedule        string   `json:"schedule"`
	IsMonitoring    bool     `json:"is_monitoring"`
	Nickname        string   `json:"nickname"`
	Avatar          string   `json:"avatar"`
	QualityMode     string   `json:"quality_mode"`
	ContainerMode   string   `json:"container_mode"`
	AuthMode        string   `json:"auth_mode"`
	TelegramEnabled bool     `json:"telegram_enabled"`
	LastError       string   `json:"last_error"`
	LastFilePath    string   `json:"last_file_path"`
	RecentLogs      []string `json:"recent_logs"`
	CurrentStatus   string   `json:"current_status"`
	LastMessage     string   `json:"last_message"`
}

func (a *App) getRuntimeStatus(screenID string) StreamerRuntimeStatus {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.runtimeStatus[normalizeID(screenID)]
}

func (a *App) GetStreamers() []StreamerStatus {
	a.mu.RLock()
	cfg := a.config
	a.mu.RUnlock()

	if cfg == nil {
		return []StreamerStatus{}
	}

	result := make([]StreamerStatus, 0, len(cfg.Streamers))
	for _, s := range cfg.Streamers {
		isMonitoring := true
		if a.scheduler != nil {
			isMonitoring = a.scheduler.IsMonitoring(s.ScreenID)
		}

		diag := a.GetStreamerDiagnostics(s.ScreenID)
		runtimeStatus := a.getRuntimeStatus(s.ScreenID)
		if runtimeStatus.Status == "" {
			if isMonitoring {
				runtimeStatus.Status = "monitoring"
				runtimeStatus.Message = "Monitoring for live stream..."
			} else {
				runtimeStatus.Status = "idle"
				runtimeStatus.Message = "Monitoring paused"
			}
		}
		result = append(result, StreamerStatus{
			ScreenID:        s.ScreenID,
			Schedule:        s.Schedule,
			IsMonitoring:    isMonitoring,
			Nickname:        s.Nickname,
			Avatar:          s.Avatar,
			QualityMode:     s.QualityMode,
			ContainerMode:   s.ContainerMode,
			AuthMode:        s.AuthMode,
			TelegramEnabled: s.TelegramEnabled,
			LastError:       diag.LastError,
			LastFilePath:    diag.LastFilePath,
			RecentLogs:      diag.RecentLogs,
			CurrentStatus:   runtimeStatus.Status,
			LastMessage:     runtimeStatus.Message,
		})
	}
	return result
}

func (a *App) GetStreamerMetadata(screenID string) *metadata.StreamerMetadata {
	meta, err := metadata.FetchMetadata(screenID)
	if err != nil {
		log.Printf("Error fetching metadata for %s: %v", screenID, err)
		return nil
	}
	return meta
}

func normalizeID(screenID string) string {
	return strings.TrimSpace(screenID)
}

func idsEqual(aID, bID string) bool {
	return strings.EqualFold(normalizeID(aID), normalizeID(bID))
}

func (a *App) AddStreamer(screenID string, schedule string, qualityMode string, containerMode string, authMode string) string {
	screenID = normalizeID(screenID)
	if screenID == "" {
		return "screen_id is empty"
	}

	if schedule == "" {
		schedule = "*/1 * * * *"
	}

	a.mu.RLock()
	cfg := a.config
	a.mu.RUnlock()
	if cfg == nil {
		return "Config not loaded"
	}

	for _, s := range cfg.Streamers {
		if idsEqual(s.ScreenID, screenID) {
			return "Streamer already exists"
		}
	}

	newStreamer := config.StreamerConfig{
		ScreenID:      screenID,
		Schedule:      schedule,
		QualityMode:   strings.ToLower(strings.TrimSpace(qualityMode)),
		ContainerMode: strings.ToLower(strings.TrimSpace(containerMode)),
		AuthMode:      strings.ToLower(strings.TrimSpace(authMode)),
	}
	if meta, err := metadata.FetchMetadata(screenID); err == nil {
		newStreamer.Nickname = meta.Nickname
		newStreamer.Avatar = meta.Avatar
	} else {
		log.Printf("Failed to fetch metadata for %s: %v", screenID, err)
	}

	if err := a.scheduler.AddStreamer(newStreamer); err != nil {
		return fmt.Sprintf("Error adding to scheduler: %v", err)
	}

	a.mu.Lock()
	a.config.Streamers = append(a.config.Streamers, newStreamer)
	err := config.SaveConfig("config.yaml", a.config)
	a.mu.Unlock()
	if err != nil {
		_ = a.scheduler.RemoveStreamer(screenID)
		return fmt.Sprintf("Error saving config: %v", err)
	}

	a.scheduler.SetMonitoring(screenID, true)
	return ""
}

func (a *App) UpdateStreamerOptions(screenID string, qualityMode string, containerMode string, authMode string, telegramEnabled bool) string {
	screenID = normalizeID(screenID)
	if screenID == "" {
		return "screen_id is empty"
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.config == nil {
		return "Config not loaded"
	}

	for i := range a.config.Streamers {
		if !idsEqual(a.config.Streamers[i].ScreenID, screenID) {
			continue
		}

		a.config.Streamers[i].QualityMode = strings.ToLower(strings.TrimSpace(qualityMode))
		a.config.Streamers[i].ContainerMode = strings.ToLower(strings.TrimSpace(containerMode))
		a.config.Streamers[i].AuthMode = strings.ToLower(strings.TrimSpace(authMode))
		a.config.Streamers[i].TelegramEnabled = telegramEnabled

		if err := config.SaveConfig("config.yaml", a.config); err != nil {
			return fmt.Sprintf("Error saving config: %v", err)
		}
		if a.scheduler != nil {
			if err := a.scheduler.AddStreamer(a.config.Streamers[i]); err != nil {
				return fmt.Sprintf("Error updating scheduler: %v", err)
			}
		}
		return ""
	}

	return "Streamer not found"
}

func (a *App) RemoveStreamer(screenID string) string {
	screenID = normalizeID(screenID)
	if screenID == "" {
		return "screen_id is empty"
	}

	if a.scheduler != nil {
		if err := a.scheduler.RemoveStreamer(screenID); err != nil {
			return fmt.Sprintf("Error removing from scheduler: %v", err)
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for i, s := range a.config.Streamers {
		if idsEqual(s.ScreenID, screenID) {
			a.config.Streamers = append(a.config.Streamers[:i], a.config.Streamers[i+1:]...)
			if err := config.SaveConfig("config.yaml", a.config); err != nil {
				return fmt.Sprintf("Error saving config: %v", err)
			}
			a.statusMu.Lock()
			delete(a.runtimeStatus, screenID)
			a.statusMu.Unlock()
			return ""
		}
	}
	return "Streamer not found"
}

func (a *App) ToggleMonitoring(screenID string) bool {
	if a.scheduler == nil {
		return false
	}
	return a.scheduler.ToggleMonitoring(normalizeID(screenID))
}

func (a *App) SetMonitoring(screenID string, monitoring bool) {
	screenID = normalizeID(screenID)
	if screenID == "" || a.scheduler == nil {
		return
	}
	a.scheduler.SetMonitoring(screenID, monitoring)
}
func (a *App) SetAllMonitoring(monitoring bool) {
	if a.scheduler == nil {
		return
	}

	a.mu.RLock()
	streamers := make([]config.StreamerConfig, len(a.config.Streamers))
	copy(streamers, a.config.Streamers)
	staggerSeconds := a.config.Recording.StartupStaggerSeconds
	a.mu.RUnlock()
	targets := streamers[:0]
	for _, streamer := range streamers {
		if a.scheduler.IsMonitoring(streamer.ScreenID) != monitoring {
			targets = append(targets, streamer)
		}
	}
	streamers = targets
	operation := a.bulkOperation.Add(1)

	if monitoring && staggerSeconds > 0 && len(streamers) > 1 {
		go func() {
			for i, s := range streamers {
				if i > 0 {
					time.Sleep(time.Duration(staggerSeconds) * time.Second)
				}
				if a.bulkOperation.Load() != operation {
					return
				}
				if a.scheduler != nil {
					a.scheduler.SetMonitoring(s.ScreenID, true)
				}
			}
		}()
		return
	}

	for _, s := range streamers {
		a.scheduler.SetMonitoring(s.ScreenID, monitoring)
	}
}

func (a *App) OpenFolder(filePath string) {
	if filePath == "" {
		return
	}
	dir := filepath.Dir(filePath)

	var cmd *exec.Cmd
	if stdruntime.GOOS == "windows" {
		cmd = exec.Command("explorer", dir)
	} else if stdruntime.GOOS == "darwin" {
		cmd = exec.Command("open", dir)
	} else {
		cmd = exec.Command("xdg-open", dir)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error opening folder: %v", err)
	}
}

func (a *App) OpenFile(filePath string) string {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "file path is empty"
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Sprintf("file not found: %v", err)
	}
	if info.IsDir() {
		return "refuse to open a directory as file"
	}

	var cmd *exec.Cmd
	if stdruntime.GOOS == "windows" {
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", filePath)
	} else if stdruntime.GOOS == "darwin" {
		cmd = exec.Command("open", filePath)
	} else {
		cmd = exec.Command("xdg-open", filePath)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("open file failed: %v", err)
	}
	return ""
}

func (a *App) OpenStreamerLogFolder(screenID string) {
	screenID = normalizeID(screenID)
	if screenID == "" {
		return
	}
	logDir := "logs"
	a.mu.RLock()
	if a.config != nil && strings.TrimSpace(a.config.OutputDirectory) != "" {
		logDir = filepath.Join(a.config.OutputDirectory, "logs")
	}
	a.mu.RUnlock()
	_ = os.MkdirAll(logDir, 0755)

	var cmd *exec.Cmd
	if stdruntime.GOOS == "windows" {
		cmd = exec.Command("explorer", logDir)
	} else if stdruntime.GOOS == "darwin" {
		cmd = exec.Command("open", logDir)
	} else {
		cmd = exec.Command("xdg-open", logDir)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Error opening log folder: %v", err)
	}
}

func (a *App) GetConfig() *config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.config == nil {
		return nil
	}

	copyCfg := *a.config
	copyCfg.Streamers = append([]config.StreamerConfig(nil), a.config.Streamers...)
	return &copyCfg
}

func (a *App) syncSchedulerAuthLocked() {
	if a.scheduler == nil || a.config == nil {
		return
	}
	a.scheduler.SetOutputDirectory(a.config.OutputDirectory)
	a.scheduler.SetAuthConfig(scheduler.AuthConfig{
		Mode:          a.config.AuthMode,
		AccessToken:   a.config.OAuth.AccessToken,
		CookieEnabled: a.config.Cookies.Enabled,
		CookieFile:    a.config.Cookies.FilePath,
	})
	a.scheduler.SetRecordingConfig(scheduler.RecordingConfig{
		QualityMode:                a.config.Recording.QualityMode,
		ContainerMode:              a.config.Recording.ContainerMode,
		SaveInfoText:               a.config.Recording.SaveInfoText,
		SaveCommentsText:           a.config.Recording.SaveCommentsText,
		SaveCommentsTextFile:       a.config.Recording.SaveCommentsTextFile,
		CommentTextTemplate:        a.config.Recording.CommentTextTemplate,
		MinDurationSeconds:         a.config.Recording.MinDurationSeconds,
		MinFileSizeMB:              a.config.Recording.MinFileSizeMB,
		StartupStaggerSeconds:      a.config.Recording.StartupStaggerSeconds,
		FFmpegPath:                 a.config.Recording.FFmpegPath,
		FFprobePath:                a.config.Recording.FFprobePath,
		WorkerEnabled:              a.config.Recording.WorkerEnabled,
		WorkerPath:                 a.config.Recording.WorkerPath,
		WorkerCheckIntervalSeconds: a.config.Recording.WorkerCheckIntervalSeconds,
		WorkerMaxRestarts:          a.config.Recording.WorkerMaxRestarts,
		ProxyEnabled:               a.config.Proxy.Enabled,
		ProxyURL:                   a.config.Proxy.URL,
	})
	a.scheduler.SetNotificationsConfig(a.config.Notifications)
	proxyURL := ""
	if a.config.Proxy.Enabled {
		proxyURL = a.config.Proxy.URL
	}
	auth.SetProxyURL(proxyURL)
	metadata.SetProxyURL(proxyURL)
}

func (a *App) SaveSettings(payload SettingsPayload) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.config == nil {
		return "Config not loaded"
	}

	if strings.TrimSpace(payload.OutputDirectory) == "" {
		payload.OutputDirectory = "."
	}
	if strings.TrimSpace(payload.AuthMode) == "" {
		payload.AuthMode = "auto"
	}
	if strings.TrimSpace(payload.Cookies.FilePath) == "" {
		payload.Cookies.FilePath = "cookies.txt"
	}
	if strings.TrimSpace(payload.Recording.QualityMode) == "" {
		payload.Recording.QualityMode = "stable"
	}
	if strings.TrimSpace(payload.Recording.ContainerMode) == "" {
		payload.Recording.ContainerMode = "mkv"
	}
	if payload.Recording.StartupStaggerSeconds < 0 {
		payload.Recording.StartupStaggerSeconds = 0
	}
	if payload.Recording.WorkerCheckIntervalSeconds <= 0 {
		payload.Recording.WorkerCheckIntervalSeconds = 30
	}
	if payload.Recording.WorkerMaxRestarts <= 0 {
		payload.Recording.WorkerMaxRestarts = 8
	}
	if payload.Recording.MinDurationSeconds <= 0 {
		payload.Recording.MinDurationSeconds = 10
	}
	if payload.Recording.MinFileSizeMB < 0 {
		payload.Recording.MinFileSizeMB = 0
	}
	payload.Proxy.URL = strings.TrimSpace(payload.Proxy.URL)
	if payload.Proxy.URL == "" {
		payload.Proxy.Enabled = false
	}
	reloadWorkers := a.config.OutputDirectory != payload.OutputDirectory ||
		a.config.AuthMode != strings.ToLower(strings.TrimSpace(payload.AuthMode)) ||
		a.config.OAuth.AccessToken != payload.OAuth.AccessToken ||
		a.config.Cookies != payload.Cookies ||
		a.config.Recording != payload.Recording ||
		a.config.Proxy != payload.Proxy

	a.config.OutputDirectory = payload.OutputDirectory
	a.config.AuthMode = strings.ToLower(strings.TrimSpace(payload.AuthMode))
	a.config.OAuth = payload.OAuth
	a.config.Cookies = payload.Cookies
	a.config.Recording = payload.Recording
	a.config.Proxy = payload.Proxy
	a.config.Notifications = payload.Notifications

	if err := config.SaveConfig("config.yaml", a.config); err != nil {
		return fmt.Sprintf("Error saving config: %v", err)
	}
	a.syncSchedulerAuthLocked()
	if reloadWorkers && a.scheduler != nil {
		a.scheduler.ReloadIdleWorkers()
	}
	return ""
}

func (a *App) TestTelegramNotification(telegram config.TelegramConfig) string {
	telegram.BotToken = strings.TrimSpace(telegram.BotToken)
	telegram.ChatID = strings.TrimSpace(telegram.ChatID)
	if telegram.BotToken == "" {
		return "Bot Token is empty"
	}
	if telegram.ChatID == "" {
		return "Chat ID is empty"
	}
	telegram.Enabled = true

	proxyURL := ""
	a.mu.RLock()
	if a.config != nil && a.config.Proxy.Enabled {
		proxyURL = strings.TrimSpace(a.config.Proxy.URL)
	}
	a.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	message := "TwitCasting Recorder 测试推送\n如果你看到这条消息，Telegram 推送已可用。"
	if err := notify.SendTelegram(ctx, telegram, proxyURL, message); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) GetOAuthAuthorizeURL() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.config == nil {
		return ""
	}
	return auth.BuildAuthorizeURL(a.config.OAuth.ClientID, a.config.OAuth.RedirectURI, "")
}

func (a *App) ExchangeOAuthCode(code string) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.config == nil {
		return "Config not loaded"
	}
	if strings.TrimSpace(code) == "" {
		return "authorization code is empty"
	}
	if strings.TrimSpace(a.config.OAuth.ClientID) == "" {
		return "Client ID is empty"
	}
	if strings.TrimSpace(a.config.OAuth.ClientSecret) == "" {
		return "Client Secret is empty"
	}
	tr, err := auth.ExchangeCode(a.config.OAuth.ClientID, a.config.OAuth.ClientSecret, a.config.OAuth.RedirectURI, code)
	if err != nil {
		return fmt.Sprintf("exchange code failed: %v. 请确认 Client ID、Client Secret、Redirect URI 与开发者平台完全一致，并使用刚刚授权得到且未使用过的新 code。", err)
	}

	a.config.OAuth.AccessToken = tr.AccessToken
	a.config.OAuth.TokenType = tr.TokenType
	a.config.OAuth.Scope = tr.Scope
	if err := config.SaveConfig("config.yaml", a.config); err != nil {
		return fmt.Sprintf("save token failed: %v", err)
	}
	a.syncSchedulerAuthLocked()
	return ""
}

func (a *App) VerifyOAuthToken() string {
	a.mu.RLock()
	token := ""
	if a.config != nil {
		token = a.config.OAuth.AccessToken
	}
	a.mu.RUnlock()

	if err := auth.VerifyAccessToken(token); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) SelectDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Output Directory",
	})
}

func (a *App) SelectCookieFile() (string, error) {
	defaultDir := "."
	a.mu.RLock()
	if a.config != nil {
		cookiePath := strings.TrimSpace(a.config.Cookies.FilePath)
		if cookiePath != "" {
			if abs, err := filepath.Abs(cookiePath); err == nil {
				defaultDir = filepath.Dir(abs)
			}
		}
	}
	a.mu.RUnlock()
	if info, err := os.Stat(defaultDir); err != nil || !info.IsDir() {
		defaultDir = "."
	}

	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Select cookies.txt",
		DefaultDirectory: defaultDir,
		DefaultFilename:  "cookies.txt",
		Filters: []runtime.FileFilter{
			{DisplayName: "Cookies text file (*.txt)", Pattern: "*.txt"},
			{DisplayName: "All files (*.*)", Pattern: "*.*"},
		},
	})
}
