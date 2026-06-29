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

func TestApplyDefaultsAcceptsExpandedQualityModes(t *testing.T) {
	for _, mode := range []string{"original", "high", "stable", "medium", "low", "auto", "audio"} {
		cfg := Config{Recording: RecordingConfig{QualityMode: mode}}
		applyDefaults(&cfg)
		want := mode
		if mode == "medium" {
			want = "medium"
		}
		if cfg.Recording.QualityMode != want {
			t.Fatalf("QualityMode %q normalized to %q, want %q", mode, cfg.Recording.QualityMode, want)
		}
	}
}

func TestNormalizeQualityModeRejectsUnknown(t *testing.T) {
	if got := normalizeQualityMode("giant"); got != "" {
		t.Fatalf("normalizeQualityMode(giant) = %q, want empty", got)
	}
	if got := normalizeQualityMode("low"); got != "low" {
		t.Fatalf("normalizeQualityMode(low) = %q, want low", got)
	}
}
