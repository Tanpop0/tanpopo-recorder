package config

import "testing"

func TestApplyDefaultsPreservesZeroStartupStagger(t *testing.T) {
	cfg := Config{
		Recording: RecordingConfig{
			StartupStaggerSeconds: 0,
		},
	}

	applyDefaults(&cfg)

	if cfg.Recording.StartupStaggerSeconds != 0 {
		t.Fatalf("StartupStaggerSeconds = %d, want 0", cfg.Recording.StartupStaggerSeconds)
	}
}
