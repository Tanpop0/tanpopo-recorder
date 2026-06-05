package recorder

import "testing"

func TestPickStableVariantURLKeepsOriginalMode(t *testing.T) {
	raw := "https://twitcasting.tv/livehls/streams/1/hls/705.00/media.705.m3u8"

	got := pickStableVariantURL(raw, "example", "original")
	if got != raw {
		t.Fatalf("pickStableVariantURL() = %q, want original %q", got, raw)
	}
}

func TestShouldForwardProcessLogFiltersProgressNoise(t *testing.T) {
	noisy := []string{
		"progress=continue",
		"frame= 8240 fps=15 q=-1.0 size= 13824KiB time=00:09:09.03 bitrate=206.3kbits/s",
		"[hls @ 000000] Skip ('#EXT-X-VERSION:3')",
	}
	for _, line := range noisy {
		if shouldForwardProcessLog(line) {
			t.Fatalf("shouldForwardProcessLog(%q) = true, want false", line)
		}
	}
}

func TestShouldForwardProcessLogKeepsActionableErrors(t *testing.T) {
	important := []string{
		"HTTP error 403 Forbidden",
		"Connection failed: input/output error",
		"Will reconnect at 1 in 5 second(s).",
	}
	for _, line := range important {
		if !shouldForwardProcessLog(line) {
			t.Fatalf("shouldForwardProcessLog(%q) = false, want true", line)
		}
	}
}

func TestResolveContainerPlan(t *testing.T) {
	tests := []struct {
		mode         string
		wantFinalExt string
		wantFormat   string
		wantRemux    bool
	}{
		{mode: "", wantFinalExt: ".mkv", wantFormat: "matroska"},
		{mode: "ts", wantFinalExt: ".ts", wantFormat: "mpegts"},
		{mode: "mp4", wantFinalExt: ".mp4", wantFormat: "mpegts", wantRemux: true},
	}

	for _, tt := range tests {
		got := resolveContainerPlan(tt.mode)
		if got.finalExt != tt.wantFinalExt || got.ffmpegFormat != tt.wantFormat || got.remuxToMP4 != tt.wantRemux {
			t.Fatalf("resolveContainerPlan(%q) = %+v", tt.mode, got)
		}
	}
}

func TestDetectLoginPlaceholderRecordingIgnoresMissingProbeData(t *testing.T) {
	if err := detectLoginPlaceholderRecording("", 0, ""); err != nil {
		t.Fatalf("detectLoginPlaceholderRecording() = %v, want nil", err)
	}
}
