package recorder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestBuildFFmpegHTTPOptionsUsesCookieJarWithoutCookieHeader(t *testing.T) {
	cookieFile := filepath.Join(t.TempDir(), "cookies.txt")
	data := ".twitcasting.tv\tTRUE\t/\tFALSE\t4102444800\ttc_ss\tsecret\n"
	if err := os.WriteFile(cookieFile, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	headerArg, cookieJar := buildFFmpegHTTPOptions("example", AuthSettings{
		Mode:          "cookie",
		CookieEnabled: true,
		CookieFile:    cookieFile,
	})

	if strings.Contains(strings.ToLower(headerArg), "cookie:") {
		t.Fatalf("buildFFmpegHTTPOptions() added an explicit Cookie header: %q", headerArg)
	}
	if !strings.Contains(cookieJar, "tc_ss=secret") {
		t.Fatalf("buildFFmpegHTTPOptions() cookie jar = %q, want tc_ss cookie", cookieJar)
	}
}

func TestIsRestrictedAccessError(t *testing.T) {
	for _, message := range []string{
		"Server returned 401 Unauthorized",
		"Server returned 403 Forbidden",
		"Server returned 404 Not Found",
	} {
		if !IsRestrictedAccessError(fmt.Errorf("%s", message)) {
			t.Fatalf("IsRestrictedAccessError(%q) = false, want true", message)
		}
	}
	if IsRestrictedAccessError(fmt.Errorf("connection reset by peer")) {
		t.Fatal("IsRestrictedAccessError(network error) = true, want false")
	}
}

func TestResolvePreferredToolPathAcceptsBinDirectory(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	ffmpegPath := filepath.Join(binDir, "ffmpeg.exe")
	if err := os.WriteFile(ffmpegPath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	got := resolvePreferredToolPath(binDir, "ffmpeg")
	if got != ffmpegPath {
		t.Fatalf("resolvePreferredToolPath() = %q, want %q", got, ffmpegPath)
	}
}

func TestResolvePreferredToolPathAcceptsExtractedRootDirectory(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	ffprobePath := filepath.Join(binDir, "ffprobe.exe")
	if err := os.WriteFile(ffprobePath, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}

	got := resolvePreferredToolPath(root, "ffprobe")
	if got != ffprobePath {
		t.Fatalf("resolvePreferredToolPath() = %q, want %q", got, ffprobePath)
	}
}

func TestDetectLoginPlaceholderRecordingIgnoresMissingProbeData(t *testing.T) {
	if err := detectLoginPlaceholderRecording("", 0, ""); err != nil {
		t.Fatalf("detectLoginPlaceholderRecording() = %v, want nil", err)
	}
}
