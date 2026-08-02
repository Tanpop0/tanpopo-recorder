package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/metadata"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/paths"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const metadataRefreshInterval = 24 * time.Hour

// RefreshStreamerMetadata refreshes one streamer on demand. The old profile is
// retained when TwitCasting or the configured proxy is temporarily unavailable.
func (a *App) RefreshStreamerMetadata(screenID string) string {
	return a.refreshStreamerMetadata(normalizeID(screenID), true)
}

func (a *App) refreshStaleStreamerMetadata() {
	a.mu.RLock()
	if a.config == nil {
		a.mu.RUnlock()
		return
	}
	streamers := append([]config.StreamerConfig(nil), a.config.Streamers...)
	a.mu.RUnlock()

	for _, streamer := range streamers {
		if a.ctx != nil {
			select {
			case <-a.ctx.Done():
				return
			default:
			}
		}
		stale := streamer.MetadataUpdatedAt.IsZero() || time.Since(streamer.MetadataUpdatedAt) >= metadataRefreshInterval
		if stale || strings.TrimSpace(streamer.Avatar) == "" || strings.TrimSpace(streamer.Nickname) == "" {
			_ = a.refreshStreamerMetadata(streamer.ScreenID, false)
		} else if !a.hasAvatarCache(streamer.ScreenID) {
			_ = a.cacheStreamerAvatar(streamer.ScreenID, streamer.Avatar)
		}
		time.Sleep(750 * time.Millisecond)
	}
}

func (a *App) refreshStreamerMetadata(screenID string, userRequested bool) string {
	if screenID == "" {
		return "主播 ID 为空"
	}
	if _, loaded := a.metadataRefreshes.LoadOrStore(strings.ToLower(screenID), struct{}{}); loaded {
		return ""
	}
	defer a.metadataRefreshes.Delete(strings.ToLower(screenID))

	meta, err := metadata.FetchUserMetadata(screenID)
	if err != nil {
		if userRequested {
			return fmt.Sprintf("刷新主播资料失败: %v", err)
		}
		log.Printf("Background metadata refresh failed for %s: %v", screenID, err)
		return ""
	}

	a.mu.Lock()
	if a.config == nil {
		a.mu.Unlock()
		return "配置尚未加载"
	}
	var updated config.StreamerConfig
	found := false
	for i := range a.config.Streamers {
		if !idsEqual(a.config.Streamers[i].ScreenID, screenID) {
			continue
		}
		if strings.TrimSpace(meta.Nickname) != "" {
			a.config.Streamers[i].Nickname = meta.Nickname
		}
		if strings.TrimSpace(meta.Avatar) != "" {
			a.config.Streamers[i].Avatar = meta.Avatar
		}
		a.config.Streamers[i].MetadataUpdatedAt = time.Now()
		updated = a.config.Streamers[i]
		found = true
		break
	}
	if !found {
		a.mu.Unlock()
		return "未找到主播"
	}
	err = config.SaveConfig(paths.DefaultConfigPath(), a.config)
	a.mu.Unlock()
	if err != nil {
		return fmt.Sprintf("保存主播资料失败: %v", err)
	}

	if strings.TrimSpace(updated.Avatar) != "" {
		if cacheErr := a.cacheStreamerAvatar(updated.ScreenID, updated.Avatar); cacheErr != nil {
			log.Printf("Avatar cache refresh failed for %s: %v", screenID, cacheErr)
		}
	}
	if a.scheduler != nil {
		_ = a.scheduler.AddStreamer(updated)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "streamer-metadata-updated", map[string]string{"screen_id": updated.ScreenID})
	}
	return ""
}

func (a *App) hasAvatarCache(screenID string) bool {
	cachePath := filepath.Join(paths.AvatarCacheDir(paths.DefaultConfigPath()), sanitizeLogName(screenID)+".img")
	info, err := os.Stat(cachePath)
	return err == nil && info.Size() > 0 && info.Size() <= 2*1024*1024
}

func (a *App) cacheStreamerAvatar(screenID, avatarURL string) error {
	data, mimeType, err := metadata.FetchAvatar(avatarURL)
	if err != nil {
		return err
	}
	dir := paths.AvatarCacheDir(paths.DefaultConfigPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	cachePath := filepath.Join(dir, sanitizeLogName(screenID)+".img")
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return err
	}
	a.avatarMu.Lock()
	a.avatarData[normalizeID(screenID)] = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	a.avatarMu.Unlock()
	return nil
}

func (a *App) avatarSource(screenID, fallbackURL string) string {
	key := normalizeID(screenID)
	a.avatarMu.RLock()
	dataURI := a.avatarData[key]
	a.avatarMu.RUnlock()
	if dataURI != "" {
		return dataURI
	}
	cachePath := filepath.Join(paths.AvatarCacheDir(paths.DefaultConfigPath()), sanitizeLogName(screenID)+".img")
	data, err := os.ReadFile(cachePath)
	if err != nil || len(data) == 0 || len(data) > 2*1024*1024 {
		return normalizeAvatarSource(fallbackURL)
	}
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return normalizeAvatarSource(fallbackURL)
	}
	dataURI = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	a.avatarMu.Lock()
	a.avatarData[key] = dataURI
	a.avatarMu.Unlock()
	return dataURI
}

func normalizeAvatarSource(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}
