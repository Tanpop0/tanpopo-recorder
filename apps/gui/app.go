package main

import (
	"context"
	"log"
	"os"
	stdruntime "runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/auth"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/history"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/metadata"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/paths"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/scheduler"

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

type RuntimeDiagnostics struct {
	ConfigPath      string `json:"config_path"`
	HistoryPath     string `json:"history_path"`
	LogsDir         string `json:"logs_dir"`
	OutputDirectory string `json:"output_directory"`
	StreamerCount   int    `json:"streamer_count"`
	WorkerEnabled   bool   `json:"worker_enabled"`
	ProxyEnabled    bool   `json:"proxy_enabled"`
	OAuthConfigured bool   `json:"oauth_configured"`
	CookieEnabled   bool   `json:"cookie_enabled"`
	CookiePath      string `json:"cookie_path"`
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

	configPath := paths.DefaultConfigPath()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Printf("Error loading config: %v. Creating default.", err)
		cfg = &config.Config{
			Streamers:       []config.StreamerConfig{},
			OutputDirectory: ".",
			AuthMode:        "auto",
			Cookies:         config.CookieConfig{Enabled: false, FilePath: paths.DefaultCookiesPath()},
			Recording:       config.RecordingConfig{QualityMode: "stable", ContainerMode: "mkv", SaveCommentsText: true, CommentTextTemplate: "[{offset}] {display_name}: {message}", StartupStaggerSeconds: 2, WorkerCheckIntervalSeconds: 30, WorkerMaxRestarts: 8},
			Notifications:   config.NotificationsConfig{Telegram: config.TelegramConfig{NotifyOnStart: true, NotifyOnFinish: true, NotifyOnError: true}},
		}
		if errCreate := config.SaveConfig(configPath, cfg); errCreate != nil {
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
	a.historyManager = history.NewHistoryManager(paths.HistoryPath(configPath))

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
