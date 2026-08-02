package scheduler

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (m *ValidationManager) recordingStartMessage(screenID, title string) string {
	displayName := m.streamerDisplayName(screenID)
	lines := []string{
		fmt.Sprintf("%s 开始直播，并已开始录制", displayName),
		fmt.Sprintf("主播: %s", displayName),
	}
	if strings.TrimSpace(title) != "" {
		lines = append(lines, fmt.Sprintf("标题: %s", strings.TrimSpace(title)))
	}
	lines = append(lines, "时间: "+time.Now().Format("2006-01-02 15:04:05"))
	return strings.Join(lines, "\n")
}

func (m *ValidationManager) recordingFinishMessage(screenID, title, duration, filePath string, fileSize int64) string {
	lines := []string{
		"TwitCasting 录制完成",
		fmt.Sprintf("主播: %s", m.streamerDisplayName(screenID)),
	}
	if strings.TrimSpace(title) != "" {
		lines = append(lines, fmt.Sprintf("标题: %s", strings.TrimSpace(title)))
	}
	if strings.TrimSpace(duration) != "" {
		lines = append(lines, "时长: "+strings.TrimSpace(duration))
	}
	if fileSize > 0 {
		lines = append(lines, "大小: "+formatBytes(fileSize))
	}
	if strings.TrimSpace(filePath) != "" {
		lines = append(lines, "文件: "+filepath.Base(filePath))
	}
	return strings.Join(lines, "\n")
}

func (m *ValidationManager) recordingErrorMessage(screenID, title, errMessage string) string {
	lines := []string{
		"TwitCasting 录制失败",
		fmt.Sprintf("主播: %s", m.streamerDisplayName(screenID)),
	}
	if strings.TrimSpace(title) != "" {
		lines = append(lines, fmt.Sprintf("标题: %s", strings.TrimSpace(title)))
	}
	lines = append(lines,
		"错误: "+strings.TrimSpace(errMessage),
		"时间: "+time.Now().Format("2006-01-02 15:04:05"),
	)
	return strings.Join(lines, "\n")
}

func (m *ValidationManager) workerErrorMessage(screenID, errMessage string) string {
	return strings.Join([]string{
		"TwitCasting Worker 异常",
		fmt.Sprintf("主播: %s", m.streamerDisplayName(screenID)),
		"错误: " + strings.TrimSpace(errMessage),
		"时间: " + time.Now().Format("2006-01-02 15:04:05"),
	}, "\n")
}

func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unit])
	}
	return fmt.Sprintf("%.2f %s", value, units[unit])
}

func classifyRecordingStatus(duration string, fileSize int64, stoppedByUser bool, recErr error, rc RecordingConfig) string {
	if stoppedByUser {
		return "manual_stopped"
	}
	if recErr != nil {
		return "failed_" + classifyErrorCategory(recErr.Error())
	}
	if isShortRecording(duration, fileSize, rc) {
		return "short"
	}
	return "completed"
}

func isShortRecording(duration string, fileSize int64, rc RecordingConfig) bool {
	if rc.MinFileSizeMB > 0 && fileSize > 0 && fileSize < int64(rc.MinFileSizeMB)*1024*1024 {
		return true
	}
	if rc.MinDurationSeconds > 0 {
		seconds := parseDurationSeconds(duration)
		if seconds > 0 && seconds < rc.MinDurationSeconds {
			return true
		}
	}
	return false
}

func parseDurationSeconds(duration string) int {
	parts := strings.Split(strings.TrimSpace(duration), ":")
	if len(parts) != 3 {
		return 0
	}
	var h, m, s int
	if _, err := fmt.Sscanf(duration, "%d:%d:%d", &h, &m, &s); err != nil {
		return 0
	}
	if h < 0 || m < 0 || s < 0 {
		return 0
	}
	return h*3600 + m*60 + s
}

func classifyErrorCategory(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"), strings.Contains(lower, "403"), strings.Contains(lower, "protected"), strings.Contains(lower, "login-required"), strings.Contains(lower, "login required"):
		return "auth"
	case strings.Contains(lower, "proxy"), strings.Contains(lower, "socks"), strings.Contains(lower, "connection refused"):
		return "proxy"
	case strings.Contains(lower, "media progress stalled"), strings.Contains(lower, "timeout"), strings.Contains(lower, "i/o timeout"), strings.Contains(lower, "network"), strings.Contains(lower, "connection reset"), strings.Contains(lower, "connection to "), strings.Contains(lower, "connection failed"), strings.Contains(lower, "input/output error"), strings.Contains(lower, "end of file"), strings.Contains(lower, "no such host"), strings.Contains(lower, "503"), strings.Contains(lower, "429"):
		return "network"
	case strings.Contains(lower, "stream url is empty"), strings.Contains(lower, "no stream url"), strings.Contains(lower, "m3u8"), strings.Contains(lower, "404 not found"), strings.Contains(lower, "server returned 404"):
		return "stream"
	case strings.Contains(lower, "permission"), strings.Contains(lower, "access is denied"), strings.Contains(lower, "mkdir"), strings.Contains(lower, "write"):
		return "file"
	case strings.Contains(lower, "ffmpeg"), strings.Contains(lower, "exit status"), strings.Contains(lower, "invalid data"):
		return "ffmpeg"
	default:
		return "unknown"
	}
}
