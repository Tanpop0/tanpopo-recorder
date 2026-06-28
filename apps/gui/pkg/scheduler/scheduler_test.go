package scheduler

import (
	"fmt"
	"testing"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/workerproc"
)

type testNotifier struct {
	statuses []string
	history  []string
}

func (n *testNotifier) NotifyStatus(screenID, status, message string) {
	n.statuses = append(n.statuses, fmt.Sprintf("%s:%s:%s", screenID, status, message))
}

func (n *testNotifier) NotifyAppLog(message string) {}

func (n *testNotifier) AddRecordingHistory(streamerID, filePath, duration string, fileSize int64) {
	n.AddRecordingHistoryWithStatus(streamerID, filePath, duration, fileSize, "completed")
}

func (n *testNotifier) AddRecordingHistoryWithStatus(streamerID, filePath, duration string, fileSize int64, status string) {
	n.history = append(n.history, fmt.Sprintf("%s:%s", streamerID, status))
}

func TestClassifyRecordingStatusShort(t *testing.T) {
	rc := RecordingConfig{MinDurationSeconds: 10}
	got := classifyRecordingStatus("00:00:05", 5*1024*1024, false, nil, rc)
	if got != "short" {
		t.Fatalf("classifyRecordingStatus() = %q, want short", got)
	}
}

func TestClassifyRecordingStatusFailedAuth(t *testing.T) {
	rc := RecordingConfig{MinDurationSeconds: 10}
	got := classifyRecordingStatus("00:02:00", 5*1024*1024, false, fmt.Errorf("HTTP error 403 Forbidden"), rc)
	if got != "failed_auth" {
		t.Fatalf("classifyRecordingStatus() = %q, want failed_auth", got)
	}
}

func TestClassifyRecordingStatusFailedAuthPlaceholder(t *testing.T) {
	rc := RecordingConfig{MinDurationSeconds: 10}
	got := classifyRecordingStatus("01:23:03", 31*1024*1024, false, fmt.Errorf("possible login-required placeholder recording detected"), rc)
	if got != "failed_auth" {
		t.Fatalf("classifyRecordingStatus() = %q, want failed_auth", got)
	}
}

func TestClassifyRecordingStatusFailedNetworkStall(t *testing.T) {
	rc := RecordingConfig{MinDurationSeconds: 10}
	got := classifyRecordingStatus("00:03:25", 50*1024*1024, false, fmt.Errorf("ffmpeg media progress stalled: media progress stalled for 3m0s"), rc)
	if got != "failed_network" {
		t.Fatalf("classifyRecordingStatus() = %q, want failed_network", got)
	}
}

func TestClassifyRecordingStatusInterrupted(t *testing.T) {
	got := classifyRecordingStatus("00:00:01", 1, true, nil, RecordingConfig{MinDurationSeconds: 10})
	if got != "manual_stopped" {
		t.Fatalf("classifyRecordingStatus() = %q, want manual_stopped", got)
	}
}

func TestTelegramEnabledForStreamerDefaultsFalse(t *testing.T) {
	manager := NewManager(nil, nil)
	manager.streamerSettings.Store("silent_user", config.StreamerConfig{ScreenID: "silent_user"})

	if manager.telegramEnabledForStreamer("silent_user") {
		t.Fatalf("telegramEnabledForStreamer() = true, want false")
	}
}

func TestTelegramEnabledForStreamerCanBeEnabled(t *testing.T) {
	manager := NewManager(nil, nil)
	manager.streamerSettings.Store("push_user", config.StreamerConfig{ScreenID: "push_user", TelegramEnabled: true})

	if !manager.telegramEnabledForStreamer("push_user") {
		t.Fatalf("telegramEnabledForStreamer() = false, want true")
	}
}

func TestGetAuthConfigForStreamerOverridesCookie(t *testing.T) {
	manager := NewManager(nil, nil)
	manager.SetAuthConfig(AuthConfig{Mode: "auto", CookieEnabled: false, CookieFile: "cookies.txt"})
	manager.streamerSettings.Store("member_only_user", config.StreamerConfig{ScreenID: "member_only_user", AuthMode: "cookie"})

	got := manager.getAuthConfigForStreamer("member_only_user")
	if got.Mode != "cookie" || !got.CookieEnabled {
		t.Fatalf("getAuthConfigForStreamer() = %+v, want forced cookie", got)
	}
}

func TestGetAuthConfigForStreamerDisablesCookie(t *testing.T) {
	manager := NewManager(nil, nil)
	manager.SetAuthConfig(AuthConfig{Mode: "auto", CookieEnabled: true, CookieFile: "cookies.txt"})
	manager.streamerSettings.Store("public_user", config.StreamerConfig{ScreenID: "public_user", AuthMode: "no_cookie"})

	got := manager.getAuthConfigForStreamer("public_user")
	if got.Mode != "oauth" || got.CookieEnabled {
		t.Fatalf("getAuthConfigForStreamer() = %+v, want cookie disabled", got)
	}
}

func TestRecordRestrictedFailureSuppressesAfterTwoAttempts(t *testing.T) {
	manager := NewManager(nil, nil)
	if got := manager.recordRestrictedFailure("member_user", "movie:1"); got != 1 {
		t.Fatalf("first recordRestrictedFailure() = %d, want 1", got)
	}
	if manager.isRestrictedLive("member_user", "movie:1") {
		t.Fatal("live suppressed after one failure")
	}
	if got := manager.recordRestrictedFailure("member_user", "movie:1"); got != 2 {
		t.Fatalf("second recordRestrictedFailure() = %d, want 2", got)
	}
	if !manager.isRestrictedLive("member_user", "movie:1") {
		t.Fatal("live not suppressed after two failures")
	}
	if !manager.isRestrictedLive("member_user", "created:2") {
		t.Fatal("weak live session was incorrectly released")
	}
	if manager.isRestrictedLive("member_user", "movie:2") {
		t.Fatal("new explicit movie session was incorrectly suppressed")
	}
}

func TestSetRecordingConfigPreservesZeroStartupStagger(t *testing.T) {
	manager := NewManager(nil, nil)
	manager.SetRecordingConfig(RecordingConfig{StartupStaggerSeconds: 0})

	if got := manager.getRecordingConfig().StartupStaggerSeconds; got != 0 {
		t.Fatalf("StartupStaggerSeconds = %d, want 0", got)
	}
}

func TestPausedStreamerIgnoresLateWorkerStatus(t *testing.T) {
	notifier := &testNotifier{}
	manager := NewManager(notifier, nil)
	manager.pausedStreamers.Store("ruru", true)

	manager.handleWorkerEvent("ruru", workerproc.Event{Type: "status", ScreenID: "ruru", Status: "recording", Message: "Recording 00:00:03"})
	if _, ok := manager.activeRecordings.Load("ruru"); ok {
		t.Fatal("late worker status marked paused streamer as recording")
	}
	if len(notifier.statuses) != 0 {
		t.Fatalf("late worker status notified UI: %+v", notifier.statuses)
	}

	manager.handleWorkerEvent("ruru", workerproc.Event{
		Type:          "result",
		ScreenID:      "ruru",
		Duration:      "00:01:07",
		FilePath:      "ruru.mkv",
		FileSize:      1024,
		StoppedByUser: true,
	})
	if len(notifier.history) != 1 || notifier.history[0] != "ruru:manual_stopped" {
		t.Fatalf("paused result history = %+v, want manual_stopped", notifier.history)
	}
}
