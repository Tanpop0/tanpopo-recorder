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

func TestSameLiveSessionForSuppression(t *testing.T) {
	tests := []struct {
		name          string
		restrictedKey string
		currentKey    string
		want          bool
	}{
		{name: "same movie", restrictedKey: "movie:1", currentKey: "movie:1", want: true},
		{name: "new explicit movie", restrictedKey: "movie:1", currentKey: "movie:2", want: false},
		{name: "created can be unstable", restrictedKey: "created:100", currentKey: "created:200", want: true},
		{name: "fallback stays suppressed", restrictedKey: "current-live", currentKey: "created:200", want: true},
		{name: "missing current key stays suppressed", restrictedKey: "movie:1", currentKey: "", want: true},
		{name: "missing restricted key does not suppress", restrictedKey: "", currentKey: "movie:1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SameLiveSessionForSuppression(tt.restrictedKey, tt.currentKey); got != tt.want {
				t.Fatalf("SameLiveSessionForSuppression(%q, %q) = %v, want %v", tt.restrictedKey, tt.currentKey, got, tt.want)
			}
		})
	}
}

func TestProtectedLiveErrorMetadata(t *testing.T) {
	err := &ProtectedLiveError{ScreenID: "i70_o0", Title: "member live", LiveKey: "movie:123"}

	protected, ok := AsProtectedLiveError(err)
	if !ok {
		t.Fatal("AsProtectedLiveError() = false, want true")
	}
	if protected.LiveKey != "movie:123" {
		t.Fatalf("protected.LiveKey = %q, want movie:123", protected.LiveKey)
	}
	if protected.Error() != "current live is protected (is_protected=true)" {
		t.Fatalf("protected.Error() = %q", protected.Error())
	}
}
