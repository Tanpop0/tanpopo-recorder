package scheduler

import (
	"fmt"
	"testing"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
)

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
