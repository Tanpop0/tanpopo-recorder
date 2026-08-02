package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/paths"

	"gopkg.in/yaml.v3"
)

// OAuthConfig stores OAuth credentials and tokens.
type OAuthConfig struct {
	ClientID     string `yaml:"client_id" json:"client_id"`
	ClientSecret string `yaml:"client_secret" json:"client_secret"`
	RedirectURI  string `yaml:"redirect_uri" json:"redirect_uri"`
	AccessToken  string `yaml:"access_token" json:"access_token"`
	TokenType    string `yaml:"token_type,omitempty" json:"token_type,omitempty"`
	Scope        string `yaml:"scope,omitempty" json:"scope,omitempty"`
}

// CookieConfig stores cookie-based auth settings.
type CookieConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	FilePath string `yaml:"file_path" json:"file_path"`
}

// RecordingConfig stores recording stability and process settings.
type RecordingConfig struct {
	QualityMode                string `yaml:"quality_mode" json:"quality_mode"`
	ContainerMode              string `yaml:"container_mode" json:"container_mode"`
	SaveInfoText               bool   `yaml:"save_info_text" json:"save_info_text"`
	SaveCommentsText           bool   `yaml:"save_comments_text" json:"save_comments_text"`
	SaveCommentsTextFile       bool   `yaml:"save_comments_text_file" json:"save_comments_text_file"`
	CommentTextTemplate        string `yaml:"comment_text_template" json:"comment_text_template"`
	MinDurationSeconds         int    `yaml:"min_duration_seconds" json:"min_duration_seconds"`
	MinFileSizeMB              int    `yaml:"min_file_size_mb" json:"min_file_size_mb"`
	StartupStaggerSeconds      int    `yaml:"startup_stagger_seconds" json:"startup_stagger_seconds"`
	FFmpegPath                 string `yaml:"ffmpeg_path" json:"ffmpeg_path"`
	FFprobePath                string `yaml:"ffprobe_path" json:"ffprobe_path"`
	WorkerEnabled              bool   `yaml:"worker_enabled" json:"worker_enabled"`
	WorkerPath                 string `yaml:"worker_path" json:"worker_path"`
	WorkerCheckIntervalSeconds int    `yaml:"worker_check_interval_seconds" json:"worker_check_interval_seconds"`
	WorkerMaxRestarts          int    `yaml:"worker_max_restarts" json:"worker_max_restarts"`
}

// ProxyConfig stores optional HTTP proxy settings.
type ProxyConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	URL     string `yaml:"url" json:"url"`
}

// NotificationsConfig stores optional external notification settings.
type NotificationsConfig struct {
	Telegram TelegramConfig `yaml:"telegram" json:"telegram"`
}

// TelegramConfig stores Telegram bot push settings.
type TelegramConfig struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	BotToken       string `yaml:"bot_token" json:"bot_token"`
	ChatID         string `yaml:"chat_id" json:"chat_id"`
	NotifyOnStart  bool   `yaml:"notify_on_start" json:"notify_on_start"`
	NotifyOnFinish bool   `yaml:"notify_on_finish" json:"notify_on_finish"`
	NotifyOnError  bool   `yaml:"notify_on_error" json:"notify_on_error"`
}

// Config represents the application configuration.
type Config struct {
	Streamers            []StreamerConfig    `yaml:"streamers"`
	OutputDirectory      string              `yaml:"output_directory" json:"output_directory"`
	CheckIntervalSeconds int                 `yaml:"check_interval_seconds,omitempty" json:"check_interval_seconds,omitempty"`
	AuthMode             string              `yaml:"auth_mode" json:"auth_mode"`
	OAuth                OAuthConfig         `yaml:"oauth" json:"oauth"`
	Cookies              CookieConfig        `yaml:"cookies" json:"cookies"`
	Recording            RecordingConfig     `yaml:"recording" json:"recording"`
	Proxy                ProxyConfig         `yaml:"proxy" json:"proxy"`
	Notifications        NotificationsConfig `yaml:"notifications" json:"notifications"`
}

// StreamerConfig holds configuration for a single streamer.
type StreamerConfig struct {
	ScreenID          string    `yaml:"screen_id" json:"screen_id"`
	Schedule          string    `yaml:"schedule" json:"schedule"`
	Disabled          bool      `yaml:"disabled,omitempty" json:"disabled,omitempty"`
	Nickname          string    `yaml:"nickname,omitempty" json:"nickname,omitempty"`
	Avatar            string    `yaml:"avatar,omitempty" json:"avatar,omitempty"`
	MetadataUpdatedAt time.Time `yaml:"metadata_updated_at,omitempty" json:"metadata_updated_at,omitempty"`
	QualityMode       string    `yaml:"quality_mode,omitempty" json:"quality_mode,omitempty"`
	ContainerMode     string    `yaml:"container_mode,omitempty" json:"container_mode,omitempty"`
	AuthMode          string    `yaml:"auth_mode,omitempty" json:"auth_mode,omitempty"`
	TelegramEnabled   bool      `yaml:"telegram_enabled,omitempty" json:"telegram_enabled,omitempty"`
}

func applyDefaults(cfg *Config) {
	if cfg.OutputDirectory == "" {
		cfg.OutputDirectory = "."
	}
	if cfg.CheckIntervalSeconds <= 0 {
		cfg.CheckIntervalSeconds = 30
	}
	if cfg.CheckIntervalSeconds < 5 {
		cfg.CheckIntervalSeconds = 5
	}
	if cfg.CheckIntervalSeconds > 300 {
		cfg.CheckIntervalSeconds = 300
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.AuthMode))
	switch mode {
	case "oauth", "cookie", "auto":
		cfg.AuthMode = mode
	default:
		cfg.AuthMode = "auto"
	}

	if cfg.Cookies.FilePath == "" {
		cfg.Cookies.FilePath = paths.DefaultCookiesPath()
	}

	qualityMode := strings.ToLower(strings.TrimSpace(cfg.Recording.QualityMode))
	switch qualityMode {
	case "auto", "stable", "original", "high", "medium", "low", "audio":
		cfg.Recording.QualityMode = qualityMode
	default:
		cfg.Recording.QualityMode = "stable"
	}

	containerMode := strings.ToLower(strings.TrimSpace(cfg.Recording.ContainerMode))
	switch containerMode {
	case "mkv", "ts", "mp4":
		cfg.Recording.ContainerMode = containerMode
	default:
		cfg.Recording.ContainerMode = "mkv"
	}

	for i := range cfg.Streamers {
		cfg.Streamers[i].QualityMode = normalizeQualityMode(cfg.Streamers[i].QualityMode)
		cfg.Streamers[i].ContainerMode = normalizeContainerMode(cfg.Streamers[i].ContainerMode)
		cfg.Streamers[i].AuthMode = normalizeStreamerAuthMode(cfg.Streamers[i].AuthMode)
	}

	if cfg.Recording.StartupStaggerSeconds < 0 {
		cfg.Recording.StartupStaggerSeconds = 0
	}
	if cfg.Recording.StartupStaggerSeconds > 30 {
		cfg.Recording.StartupStaggerSeconds = 30
	}
	if cfg.Recording.WorkerCheckIntervalSeconds <= 0 {
		cfg.Recording.WorkerCheckIntervalSeconds = 30
	}
	if cfg.Recording.WorkerCheckIntervalSeconds < 5 {
		cfg.Recording.WorkerCheckIntervalSeconds = 5
	}
	if cfg.Recording.WorkerCheckIntervalSeconds > 300 {
		cfg.Recording.WorkerCheckIntervalSeconds = 300
	}
	if cfg.Recording.WorkerMaxRestarts <= 0 {
		cfg.Recording.WorkerMaxRestarts = 8
	}
	if cfg.Recording.WorkerMaxRestarts > 100 {
		cfg.Recording.WorkerMaxRestarts = 100
	}
	if cfg.Recording.MinDurationSeconds <= 0 {
		cfg.Recording.MinDurationSeconds = 10
	}
	if cfg.Recording.MinDurationSeconds > 3600 {
		cfg.Recording.MinDurationSeconds = 3600
	}
	if cfg.Recording.MinFileSizeMB < 0 {
		cfg.Recording.MinFileSizeMB = 0
	}
	if cfg.Recording.MinFileSizeMB > 1024 {
		cfg.Recording.MinFileSizeMB = 1024
	}
	cfg.Recording.CommentTextTemplate = strings.TrimSpace(cfg.Recording.CommentTextTemplate)
	if cfg.Recording.CommentTextTemplate == "" {
		cfg.Recording.CommentTextTemplate = "[{offset}] {display_name}: {message}"
	}
	cfg.Proxy.URL = strings.TrimSpace(cfg.Proxy.URL)
	if cfg.Proxy.URL == "" {
		cfg.Proxy.Enabled = false
	}
	cfg.Notifications.Telegram.BotToken = strings.TrimSpace(cfg.Notifications.Telegram.BotToken)
	cfg.Notifications.Telegram.ChatID = strings.TrimSpace(cfg.Notifications.Telegram.ChatID)
	if cfg.Notifications.Telegram.BotToken == "" || cfg.Notifications.Telegram.ChatID == "" {
		cfg.Notifications.Telegram.Enabled = false
	}
}

func normalizeQualityMode(value string) string {
	mode := strings.ToLower(strings.TrimSpace(value))
	switch mode {
	case "", "auto", "stable", "original", "high", "medium", "low", "audio":
		return mode
	default:
		return ""
	}
}

func normalizeContainerMode(value string) string {
	mode := strings.ToLower(strings.TrimSpace(value))
	switch mode {
	case "", "mkv", "ts", "mp4":
		return mode
	default:
		return ""
	}
}

func normalizeStreamerAuthMode(value string) string {
	mode := strings.ToLower(strings.TrimSpace(value))
	switch mode {
	case "", "inherit", "auto":
		return ""
	case "cookie", "force_cookie":
		return "cookie"
	case "no_cookie", "nocookie", "none":
		return "no_cookie"
	default:
		return ""
	}
}

// LoadConfig reads configuration from the specified file path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := string(data)
	missingSaveCommentsText := !strings.Contains(raw, "save_comments_text")
	missingTelegramStart := !strings.Contains(raw, "notify_on_start")
	missingTelegramFinish := !strings.Contains(raw, "notify_on_finish")
	missingTelegramError := !strings.Contains(raw, "notify_on_error")

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if missingTelegramStart {
		cfg.Notifications.Telegram.NotifyOnStart = true
	}
	if missingTelegramFinish {
		cfg.Notifications.Telegram.NotifyOnFinish = true
	}
	if missingTelegramError {
		cfg.Notifications.Telegram.NotifyOnError = true
	}
	applyDefaults(&cfg)
	if missingSaveCommentsText {
		cfg.Recording.SaveCommentsText = true
	}
	return &cfg, nil
}

// SaveConfig writes the configuration to the specified file path.
func SaveConfig(path string, cfg *Config) error {
	applyDefaults(cfg)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(strings.TrimSpace(path)); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0644)
}
