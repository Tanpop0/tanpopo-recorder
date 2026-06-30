package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/paths"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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
	logDir := paths.LogsDir("")
	a.mu.RLock()
	if a.config != nil && strings.TrimSpace(a.config.OutputDirectory) != "" {
		logDir = paths.LogsDir(a.config.OutputDirectory)
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

func (a *App) GetRuntimeDiagnostics() RuntimeDiagnostics {
	a.mu.RLock()
	defer a.mu.RUnlock()
	diag := RuntimeDiagnostics{
		ConfigPath:  paths.DefaultConfigPath(),
		HistoryPath: paths.HistoryPath(paths.DefaultConfigPath()),
		LogsDir:     paths.LogsDir(""),
	}
	if a.config == nil {
		return diag
	}
	diag.OutputDirectory = a.config.OutputDirectory
	diag.LogsDir = paths.LogsDir(a.config.OutputDirectory)
	diag.StreamerCount = len(a.config.Streamers)
	diag.WorkerEnabled = a.config.Recording.WorkerEnabled
	diag.ProxyEnabled = a.config.Proxy.Enabled
	diag.OAuthConfigured = strings.TrimSpace(a.config.OAuth.AccessToken) != ""
	diag.CookieEnabled = a.config.Cookies.Enabled
	diag.CookiePath = a.config.Cookies.FilePath
	return diag
}
