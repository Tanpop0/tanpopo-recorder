package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/auth"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/metadata"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/notify"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/paths"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/scheduler"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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
		payload.Cookies.FilePath = paths.DefaultCookiesPath()
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

	if err := config.SaveConfig(paths.DefaultConfigPath(), a.config); err != nil {
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
	if err := config.SaveConfig(paths.DefaultConfigPath(), a.config); err != nil {
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
	if info, err := osStatDir(defaultDir); err != nil || !info {
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

func osStatDir(path string) (bool, error) {
	info, err := os.Stat(path)
	return err == nil && info.IsDir(), err
}
