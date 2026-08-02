package history

import "strings"

// FailureInfo converts stable status codes into a concise user-facing reason.
// The original error detail remains separate and is never translated or discarded.
func FailureInfo(status, detail string) (code, summary string) {
	status = strings.ToLower(strings.TrimSpace(status))
	if !strings.HasPrefix(status, "failed") && status != "error" {
		return "", ""
	}
	code = strings.TrimPrefix(status, "failed_")
	if code == "failed" || code == "error" || code == "" {
		code = classifyDetail(detail)
	}
	switch code {
	case "auth":
		summary = "Cookie 或账号鉴权失败"
	case "proxy":
		summary = "代理连接失败"
	case "network":
		summary = "网络连接异常或媒体进度停滞"
	case "stream":
		summary = "录制流不可用、已失效或无权访问"
	case "file":
		summary = "录像文件创建或写入失败"
	case "ffmpeg":
		summary = "FFmpeg 处理录制流失败"
	case "short":
		summary = "录制失败且生成的录像过短"
	default:
		code = "unknown"
		summary = "录制异常，原因未能自动识别"
	}
	return code, summary
}

func classifyDetail(detail string) string {
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"), strings.Contains(lower, "login-required"), strings.Contains(lower, "login required"):
		return "auth"
	case strings.Contains(lower, "proxy"), strings.Contains(lower, "socks"):
		return "proxy"
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "connection reset"), strings.Contains(lower, "media progress stalled"):
		return "network"
	case strings.Contains(lower, "404"), strings.Contains(lower, "stream url"), strings.Contains(lower, "m3u8"):
		return "stream"
	case strings.Contains(lower, "permission"), strings.Contains(lower, "access is denied"), strings.Contains(lower, "write"):
		return "file"
	case strings.Contains(lower, "ffmpeg"), strings.Contains(lower, "exit status"), strings.Contains(lower, "invalid data"):
		return "ffmpeg"
	default:
		return "unknown"
	}
}
