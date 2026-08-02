package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/history"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/recorder"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) AddRecordingHistory(streamerID, filePath, duration string, fileSize int64) {
	a.AddRecordingHistoryWithDetails(streamerID, filePath, duration, fileSize, "completed", "")
}

func (a *App) AddRecordingHistoryWithStatus(streamerID, filePath, duration string, fileSize int64, status string) {
	a.AddRecordingHistoryWithDetails(streamerID, filePath, duration, fileSize, status, "")
}

func (a *App) AddRecordingHistoryWithDetails(streamerID, filePath, duration string, fileSize int64, status, errorDetail string) {
	if a.historyManager == nil {
		return
	}
	if strings.TrimSpace(status) == "" {
		status = "completed"
	}

	ffprobePath := ""
	a.mu.RLock()
	if a.config != nil {
		ffprobePath = a.config.Recording.FFprobePath
	}
	a.mu.RUnlock()
	media := recorder.ProbeMediaDetails(filePath, fileSize, duration, ffprobePath)

	now := time.Now()
	errorCode, errorSummary := history.FailureInfo(status, errorDetail)
	record := history.RecordingRecord{
		ID:           uuid.New().String(),
		StreamerID:   streamerID,
		FilePath:     filePath,
		FileSize:     fileSize,
		Duration:     duration,
		StartTime:    now,
		EndTime:      now,
		Status:       status,
		ErrorCode:    errorCode,
		ErrorSummary: errorSummary,
		ErrorDetail:  strings.TrimSpace(errorDetail),
		MediaBitrate: media.MediaBitrate,
		VideoBitrate: media.VideoBitrate,
		AudioBitrate: media.AudioBitrate,
		Width:        media.Width,
		Height:       media.Height,
		FrameRate:    media.FrameRate,
		VideoCodec:   media.VideoCodec,
		AudioCodec:   media.AudioCodec,
	}
	if errorCode != "" {
		record.ErrorAt = &now
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
		if records[i].ErrorSummary == "" {
			records[i].ErrorCode, records[i].ErrorSummary = history.FailureInfo(records[i].Status, records[i].ErrorDetail)
		}
		if meta, ok := streamers[normalizeID(records[i].StreamerID)]; ok {
			records[i].Nickname = meta.Nickname
			records[i].Avatar = a.avatarSource(records[i].StreamerID, meta.Avatar)
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
