package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/metadata"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/paths"
)

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
			Avatar:          a.avatarSource(s.ScreenID, s.Avatar),
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
		newStreamer.MetadataUpdatedAt = time.Now()
		_ = a.cacheStreamerAvatar(screenID, meta.Avatar)
	} else {
		log.Printf("Failed to fetch metadata for %s: %v", screenID, err)
	}

	if err := a.scheduler.AddStreamer(newStreamer); err != nil {
		return fmt.Sprintf("Error adding to scheduler: %v", err)
	}

	a.mu.Lock()
	a.config.Streamers = append(a.config.Streamers, newStreamer)
	err := config.SaveConfig(paths.DefaultConfigPath(), a.config)
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

		if err := config.SaveConfig(paths.DefaultConfigPath(), a.config); err != nil {
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
			if err := config.SaveConfig(paths.DefaultConfigPath(), a.config); err != nil {
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
