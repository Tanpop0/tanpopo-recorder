package recorder

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMediaProgressMonitorRejectsZeroAndDuplicateProgress(t *testing.T) {
	startedAt := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	monitor := newMediaProgressMonitor(startedAt)

	if _, advanced := monitor.Observe("00:00:00.000000", startedAt.Add(10*time.Second)); advanced {
		t.Fatal("zero media time was treated as advancing progress")
	}
	if _, advanced := monitor.Observe("00:00:03.000000", startedAt.Add(20*time.Second)); !advanced {
		t.Fatal("first positive media time was not treated as progress")
	}
	if _, advanced := monitor.Observe("00:00:03.500000", startedAt.Add(time.Minute)); advanced {
		t.Fatal("duplicate normalized media second was treated as progress")
	}
	if _, advanced := monitor.Observe("00:00:04.000000", startedAt.Add(70*time.Second)); !advanced {
		t.Fatal("increased media time was not treated as progress")
	}
}

func TestMediaProgressMonitorDetectsStartupWithoutMedia(t *testing.T) {
	startedAt := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	monitor := newMediaProgressMonitor(startedAt)

	if reason := monitor.StallReason(startedAt.Add(startupNoMediaProgressTimeout - time.Second)); reason != "" {
		t.Fatalf("startup watchdog fired early: %q", reason)
	}
	reason := monitor.StallReason(startedAt.Add(startupNoMediaProgressTimeout))
	if !strings.Contains(reason, "no advancing media progress") {
		t.Fatalf("startup watchdog reason = %q", reason)
	}
}

func TestMediaProgressMonitorDetectsRepeatedTimestampStall(t *testing.T) {
	startedAt := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	monitor := newMediaProgressMonitor(startedAt)
	firstAt := startedAt.Add(5 * time.Second)
	monitor.Observe("00:00:01", firstAt)
	monitor.Observe("00:00:01", firstAt.Add(2*time.Minute))

	reason := monitor.StallReason(firstAt.Add(mediaProgressStallTimeout))
	if !strings.Contains(reason, "did not advance") {
		t.Fatalf("duplicate timestamp stall reason = %q", reason)
	}
}

func TestMediaProgressMonitorDetectsSustainedLag(t *testing.T) {
	startedAt := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	monitor := newMediaProgressMonitor(startedAt)
	firstAt := startedAt.Add(5 * time.Second)
	monitor.Observe("00:00:01", firstAt)

	lagStartedAt := firstAt.Add(4 * time.Minute)
	monitor.Observe("00:01:00", lagStartedAt)
	if reason := monitor.StallReason(lagStartedAt); reason != "" {
		t.Fatalf("lag watchdog fired without confirmation: %q", reason)
	}
	monitor.Observe("00:01:01", lagStartedAt.Add(mediaProgressLagConfirm))
	reason := monitor.StallReason(lagStartedAt.Add(mediaProgressLagConfirm))
	if !strings.Contains(reason, "behind recording time") {
		t.Fatalf("lag watchdog reason = %q", reason)
	}
}

func TestMediaProgressMonitorAllowsNormalProgress(t *testing.T) {
	startedAt := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	monitor := newMediaProgressMonitor(startedAt)
	monitor.Observe("00:00:01", startedAt.Add(time.Second))
	monitor.Observe("00:04:00", startedAt.Add(4*time.Minute))

	if reason := monitor.StallReason(startedAt.Add(4 * time.Minute)); reason != "" {
		t.Fatalf("normal media progress was marked stalled: %q", reason)
	}
}

func TestIsMediaProgressStallError(t *testing.T) {
	if !IsMediaProgressStallError(fmt.Errorf("ffmpeg media progress stalled: media progress did not advance")) {
		t.Fatal("watchdog error was not recognized")
	}
	if IsMediaProgressStallError(fmt.Errorf("connection reset by peer")) {
		t.Fatal("ordinary network error was recognized as watchdog recycling")
	}
}
