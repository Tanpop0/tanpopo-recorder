package workerproc

// Job describes one isolated recorder-worker task.
type Job struct {
	Mode                 string        `json:"mode,omitempty"`
	ScreenID             string        `json:"screen_id"`
	Title                string        `json:"title"`
	MovieID              string        `json:"movie_id,omitempty"`
	StreamURL            string        `json:"stream_url"`
	OutputDir            string        `json:"output_dir"`
	StopFile             string        `json:"stop_file,omitempty"`
	CheckIntervalSeconds int           `json:"check_interval_seconds,omitempty"`
	Auth                 AuthSettings  `json:"auth"`
	Options              RecordOptions `json:"options"`
}

type AuthSettings struct {
	Mode          string `json:"mode"`
	AccessToken   string `json:"access_token"`
	CookieFile    string `json:"cookie_file"`
	CookieEnabled bool   `json:"cookie_enabled"`
}

type RecordOptions struct {
	QualityMode          string `json:"quality_mode"`
	ContainerMode        string `json:"container_mode"`
	SaveInfoText         bool   `json:"save_info_text"`
	SaveCommentsText     bool   `json:"save_comments_text"`
	SaveCommentsTextFile bool   `json:"save_comments_text_file"`
	CommentTextTemplate  string `json:"comment_text_template"`
	FFmpegPath           string `json:"ffmpeg_path"`
	FFprobePath          string `json:"ffprobe_path"`
	ProxyURL             string `json:"proxy_url"`
}

// Event is emitted by recorder-worker as one JSON object per stdout line.
type Event struct {
	Type          string `json:"type"`
	Time          string `json:"time"`
	ScreenID      string `json:"screen_id,omitempty"`
	Status        string `json:"status,omitempty"`
	Message       string `json:"message,omitempty"`
	Duration      string `json:"duration,omitempty"`
	FilePath      string `json:"file_path,omitempty"`
	FileSize      int64  `json:"file_size,omitempty"`
	StoppedByUser bool   `json:"stopped_by_user,omitempty"`
	Error         string `json:"error,omitempty"`
}
