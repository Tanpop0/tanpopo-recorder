package checker

import "testing"

func TestIsRecordableLiveURLAcceptsLiveHLS(t *testing.T) {
	accepted := []string{
		"https://202-234-23-233.twitcasting.tv/tc.livehls/v1/streams/835493102/hls/132.68/media_705.m3u8",
		"https://example.com/live/playlist.m3u8",
	}
	for _, url := range accepted {
		if !IsRecordableLiveURL(url) {
			t.Fatalf("IsRecordableLiveURL(%q) rejected live HLS URL", url)
		}
	}
}

func TestIsRecordableLiveURLRejectsArchiveAndFallback(t *testing.T) {
	rejected := []string{
		"",
		"https://twitcasting.tv/user/movie/123456789",
		"https://twitcasting.tv/user/archive/123456789.m3u8",
		"https://twitcasting.tv/user/metastream.m3u8",
		"https://example.com/video.mp4",
	}
	for _, url := range rejected {
		if IsRecordableLiveURL(url) {
			t.Fatalf("IsRecordableLiveURL(%q) = true, want false", url)
		}
	}
}

func TestLiveSessionKey(t *testing.T) {
	if got := LiveSessionKey(&StreamInfo{MovieID: "123", Created: 456}); got != "movie:123" {
		t.Fatalf("LiveSessionKey(movie) = %q, want movie:123", got)
	}
	if got := LiveSessionKey(&StreamInfo{Created: 456}); got != "created:456" {
		t.Fatalf("LiveSessionKey(created) = %q, want created:456", got)
	}
	if got := LiveSessionKey(&StreamInfo{}); got != "current-live" {
		t.Fatalf("LiveSessionKey(fallback) = %q, want current-live", got)
	}
}
