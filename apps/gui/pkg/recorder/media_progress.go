package recorder

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	startupNoMediaProgressTimeout = 90 * time.Second
	mediaProgressStallTimeout     = 3 * time.Minute
	mediaProgressLagThreshold     = 3 * time.Minute
	mediaProgressLagConfirm       = 45 * time.Second
)

// mediaProgressMonitor tracks actual media-time growth. Repeated FFmpeg
// progress lines with the same timestamp are deliberately not treated as
// progress.
type mediaProgressMonitor struct {
	mu sync.Mutex

	startedAt        time.Time
	sawProgress      bool
	firstProgressAt  time.Time
	firstMediaSecond int64
	lastAdvanceAt    time.Time
	lastMediaSecond  int64
	lastMediaTime    string
	lagExceededAt    time.Time
}

func newMediaProgressMonitor(startedAt time.Time) *mediaProgressMonitor {
	return &mediaProgressMonitor{startedAt: startedAt}
}

func (m *mediaProgressMonitor) Observe(raw string, now time.Time) (string, bool) {
	normalized := normalizeMediaTime(raw)
	seconds, ok := normalizedMediaSeconds(normalized)
	if !ok || seconds <= 0 {
		return normalized, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.sawProgress {
		m.sawProgress = true
		m.firstProgressAt = now
		m.firstMediaSecond = seconds
		m.lastAdvanceAt = now
		m.lastMediaSecond = seconds
		m.lastMediaTime = normalized
		return normalized, true
	}
	if seconds <= m.lastMediaSecond {
		return normalized, false
	}

	m.lastAdvanceAt = now
	m.lastMediaSecond = seconds
	m.lastMediaTime = normalized
	return normalized, true
}

func (m *mediaProgressMonitor) StallReason(now time.Time) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.sawProgress {
		if now.Sub(m.startedAt) >= startupNoMediaProgressTimeout {
			return fmt.Sprintf("no advancing media progress for %s after start", startupNoMediaProgressTimeout)
		}
		return ""
	}

	if now.Sub(m.lastAdvanceAt) >= mediaProgressStallTimeout {
		return fmt.Sprintf("media progress did not advance for %s", mediaProgressStallTimeout)
	}

	wallGrowth := now.Sub(m.firstProgressAt)
	mediaGrowth := time.Duration(m.lastMediaSecond-m.firstMediaSecond) * time.Second
	lag := wallGrowth - mediaGrowth
	if lag < mediaProgressLagThreshold {
		m.lagExceededAt = time.Time{}
		return ""
	}
	if m.lagExceededAt.IsZero() {
		m.lagExceededAt = now
		return ""
	}
	if now.Sub(m.lagExceededAt) < mediaProgressLagConfirm {
		return ""
	}
	return fmt.Sprintf("media progress is %s behind recording time", lag.Round(time.Second))
}

func (m *mediaProgressMonitor) LastMediaTime() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastMediaTime
}

func normalizedMediaSeconds(normalized string) (int64, bool) {
	var hours, minutes, seconds int64
	if _, err := fmt.Sscanf(normalized, "%d:%d:%d", &hours, &minutes, &seconds); err != nil {
		return 0, false
	}
	if hours < 0 || minutes < 0 || seconds < 0 || minutes >= 60 || seconds >= 60 {
		return 0, false
	}
	return hours*3600 + minutes*60 + seconds, true
}

// IsMediaProgressStallError reports watchdog-triggered recorder recycling.
func IsMediaProgressStallError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "ffmpeg media progress stalled")
}
