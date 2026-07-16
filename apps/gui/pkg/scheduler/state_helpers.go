package scheduler

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/checker"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
)

func isForcedCookieAuth(auth AuthConfig) bool {
	return auth.CookieEnabled && strings.EqualFold(strings.TrimSpace(auth.Mode), "cookie")
}

func forcedCookieStatusCheckJitter(screenID string) time.Duration {
	key := strings.ToLower(strings.TrimSpace(screenID))
	if key == "" {
		return 0
	}
	slots := int(forcedCookieStatusCheckJitterLimit / time.Second)
	if slots <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return time.Duration(h.Sum32()%uint32(slots)) * time.Second
}

func isTransientStatusCheckError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline exceeded") ||
		strings.Contains(lower, "awaiting headers") ||
		strings.Contains(lower, "temporary") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "no such host")
}

func (m *ValidationManager) waitForForcedCookieCheckSlot(screenID string) bool {
	delay := forcedCookieStatusCheckJitter(screenID)
	if delay <= 0 {
		return true
	}
	deadline := time.NewTimer(delay)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline.C:
			_, paused := m.pausedStreamers.Load(screenID)
			return !paused
		case <-ticker.C:
			if _, paused := m.pausedStreamers.Load(screenID); paused {
				return false
			}
			if _, recording := m.activeRecordings.Load(screenID); recording {
				return false
			}
		}
	}
}

func (m *ValidationManager) recordCheckFailure(screenID string) int {
	failures := 0
	if value, ok := m.checkFailures.Load(screenID); ok {
		if previous, okCast := value.(int); okCast {
			failures = previous
		}
	}
	failures++
	m.checkFailures.Store(screenID, failures)
	return failures
}

func (m *ValidationManager) clearCheckFailure(screenID string) {
	m.checkFailures.Delete(screenID)
}

func (m *ValidationManager) retryRecordingAfterDelay(screenID string, delay time.Duration) {
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C

		if !m.IsMonitoring(screenID) {
			return
		}
		if _, recording := m.activeRecordings.Load(screenID); recording {
			return
		}
		if m.getRecordingConfigForStreamer(screenID).WorkerEnabled {
			return
		}
		m.checkAndRecordImmediate(screenID)
	}()
}

func (m *ValidationManager) isRestrictedLive(screenID, liveKey string) bool {
	value, ok := m.restrictedLives.Load(screenID)
	if !ok {
		return false
	}
	key, ok := value.(string)
	if !ok {
		return false
	}
	if checker.SameLiveSessionForSuppression(key, liveKey) {
		return true
	}
	m.restrictedLives.Delete(screenID)
	m.restrictedChecks.Delete(screenID)
	return false
}

func (m *ValidationManager) suppressRestrictedLive(screenID, liveKey string) bool {
	liveKey = strings.TrimSpace(liveKey)
	if liveKey == "" {
		liveKey = "current-live"
	}
	if m.isRestrictedLive(screenID, liveKey) {
		return false
	}
	m.restrictedChecks.Delete(screenID)
	m.restrictedLives.Store(screenID, liveKey)
	return true
}

func (m *ValidationManager) clearRestrictedLive(screenID string) bool {
	_, existed := m.restrictedLives.LoadAndDelete(screenID)
	m.restrictedChecks.Delete(screenID)
	return existed
}

func (m *ValidationManager) recordRestrictedFailure(screenID, liveKey string) int {
	check := restrictedCheck{LiveKey: liveKey}
	if value, ok := m.restrictedChecks.Load(screenID); ok {
		if previous, okCast := value.(restrictedCheck); okCast && previous.LiveKey == liveKey {
			check = previous
		}
	}
	check.Count++
	m.restrictedChecks.Store(screenID, check)
	if check.Count >= 2 {
		m.restrictedChecks.Delete(screenID)
		m.restrictedLives.Store(screenID, liveKey)
	}
	return check.Count
}

func (m *ValidationManager) getStreamerConfig(screenID string) (config.StreamerConfig, bool) {
	if value, ok := m.streamerSettings.Load(screenID); ok {
		if streamer, okCast := value.(config.StreamerConfig); okCast {
			return streamer, true
		}
	}

	var found config.StreamerConfig
	ok := false
	m.streamerSettings.Range(func(_, value any) bool {
		streamer, okCast := value.(config.StreamerConfig)
		if !okCast {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(streamer.ScreenID), strings.TrimSpace(screenID)) {
			found = streamer
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

func (m *ValidationManager) telegramEnabledForStreamer(screenID string) bool {
	streamer, ok := m.getStreamerConfig(screenID)
	return ok && streamer.TelegramEnabled
}

func (m *ValidationManager) streamerDisplayName(screenID string) string {
	streamer, ok := m.getStreamerConfig(screenID)
	if !ok {
		return strings.TrimSpace(screenID)
	}
	nickname := strings.TrimSpace(streamer.Nickname)
	id := strings.TrimSpace(streamer.ScreenID)
	if id == "" {
		id = strings.TrimSpace(screenID)
	}
	if nickname == "" {
		return id
	}
	return fmt.Sprintf("%s / %s", nickname, id)
}
