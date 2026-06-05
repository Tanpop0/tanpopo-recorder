package workerproc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeJobDefaults(t *testing.T) {
	job := normalizeJob(Job{
		ScreenID:  " example ",
		StreamURL: " https://example.com/live.m3u8 ",
	})

	if job.ScreenID != "example" {
		t.Fatalf("ScreenID = %q, want example", job.ScreenID)
	}
	if job.Mode != "record" {
		t.Fatalf("Mode = %q, want record", job.Mode)
	}
	if job.StreamURL != "https://example.com/live.m3u8" {
		t.Fatalf("StreamURL = %q", job.StreamURL)
	}
	if job.Title != "untitled" {
		t.Fatalf("Title = %q, want untitled", job.Title)
	}
	if job.OutputDir != "." {
		t.Fatalf("OutputDir = %q, want .", job.OutputDir)
	}
	if job.Auth.Mode != "auto" {
		t.Fatalf("Auth.Mode = %q, want auto", job.Auth.Mode)
	}
	if job.Auth.CookieFile != "cookies.txt" {
		t.Fatalf("Auth.CookieFile = %q, want cookies.txt", job.Auth.CookieFile)
	}
	if job.Options.QualityMode != "stable" {
		t.Fatalf("Options.QualityMode = %q, want stable", job.Options.QualityMode)
	}
	if job.CheckIntervalSeconds != 30 {
		t.Fatalf("CheckIntervalSeconds = %d, want 30", job.CheckIntervalSeconds)
	}
}

func TestNormalizeJobAllowsMonitorWithoutStreamURL(t *testing.T) {
	job := normalizeJob(Job{
		Mode:     " MONITOR ",
		ScreenID: " example ",
	})

	if job.Mode != "monitor" {
		t.Fatalf("Mode = %q, want monitor", job.Mode)
	}
	if job.StreamURL != "" {
		t.Fatalf("StreamURL = %q, want empty", job.StreamURL)
	}
	if job.CheckIntervalSeconds != 30 {
		t.Fatalf("CheckIntervalSeconds = %d, want 30", job.CheckIntervalSeconds)
	}
}

func TestWriteJobFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job.json")

	err := writeJobFile(path, normalizeJob(Job{
		ScreenID:  "example",
		StreamURL: "https://example.com/live.m3u8",
	}))
	if err != nil {
		t.Fatalf("writeJobFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("job file is empty")
	}
}

func TestParseEventLine(t *testing.T) {
	evt, err := parseEventLine(`{"type":"status","screen_id":"example","status":"recording","message":"Recording 00:00:01"}`)
	if err != nil {
		t.Fatalf("parseEventLine() error = %v", err)
	}
	if evt.Type != "status" || evt.ScreenID != "example" || evt.Status != "recording" {
		t.Fatalf("unexpected event: %+v", evt)
	}
}

func TestParseEventLineRejectsMissingType(t *testing.T) {
	_, err := parseEventLine(`{"screen_id":"example"}`)
	if err == nil {
		t.Fatal("parseEventLine() error = nil, want error")
	}
}
