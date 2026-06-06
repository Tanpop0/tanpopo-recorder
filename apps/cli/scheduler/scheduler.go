package scheduler

import (
	"fmt"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/user/twitcasting-recorder/checker"
	"github.com/user/twitcasting-recorder/config"
	"github.com/user/twitcasting-recorder/recorder"
)

type ValidationManager struct {
	activeRecordings sync.Map // map[string]bool
	cron             *cron.Cron
}

func NewManager() *ValidationManager {
	return &ValidationManager{
		cron: cron.New(),
	}
}

func (m *ValidationManager) Start() {
	m.cron.Start()
}

func (m *ValidationManager) Stop() {
	m.cron.Stop()
}

func (m *ValidationManager) AddStreamer(streamer config.StreamerConfig) error {
	_, err := m.cron.AddFunc(streamer.Schedule, func() {
		m.checkAndRecord(streamer.ScreenID)
	})
	return err
}

func (m *ValidationManager) checkAndRecord(screenID string) {
	// Check if already recording
	if _, recording := m.activeRecordings.Load(screenID); recording {
		// fmt.Printf("Already recording %s, skipping check.\n", screenID)
		return
	}

	// Check status
	fmt.Printf("Checking status for %s...\n", screenID)
	info, err := checker.CheckStreamStatus(screenID)
	if err != nil {
		fmt.Printf("Error checking %s: %v\n", screenID, err)
		return
	}

	if info.IsLive {
		fmt.Printf("%s is LIVE! Starting recording...\n", screenID)
		m.activeRecordings.Store(screenID, true)

		// Run recording in background
		go func() {
			defer m.activeRecordings.Delete(screenID)

			// Use Streamlink wrapper
			// We don't need the WS URL anymore, just the screenID
			err := recorder.RecordStreamStreamlink(screenID, ".")
			if err != nil {
				fmt.Printf("Recording error for %s: %v\n", screenID, err)
			}
		}()
	} else {
		fmt.Printf("%s is offline.\n", screenID)
	}
}
