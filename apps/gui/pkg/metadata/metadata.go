package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type StreamerMetadata struct {
	ScreenID  string `json:"screen_id"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	IsLive    bool   `json:"is_live"`
	LiveTitle string `json:"live_title"`
}

var proxyMu sync.RWMutex
var proxyURL string

func SetProxyURL(rawURL string) {
	proxyMu.Lock()
	proxyURL = strings.TrimSpace(rawURL)
	proxyMu.Unlock()
}

func newHTTPClient(timeout time.Duration) *http.Client {
	proxyMu.RLock()
	rawProxy := proxyURL
	proxyMu.RUnlock()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if rawProxy != "" {
		if u, err := url.Parse(rawProxy); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

// FetchMetadata retrieves metadata for a given TwitCasting screen ID
// It uses the unofficial frontend API or scraping approach
// Since scraping can be fragile, we'll use a public API endpoint if available,
// or fallback to basic scraping. For TwitCasting, `https://twitcasting.tv/streamserver.php?target=[ID]&mode=client`
// returns JSON with some info, or the main page HTML meta tags.
//
// A reliable way is checking: https://frontendapi.twitcasting.tv/users/[ID]/latest-movie
// Or just parsing the main page.
func FetchMetadata(screenID string) (*StreamerMetadata, error) {
	// Let's use the frontend API for user info which is cleaner than HTML scraping
	// URL: https://frontendapi.twitcasting.tv/users/[ID]

	meta, err := FetchUserMetadata(screenID)
	if err != nil {
		return nil, err
	}
	client := newHTTPClient(10 * time.Second)

	// To check if live and get title, we might need another endpoint or the same one if it has live info.
	// Let's check the latest-movie endpoint for live status/title
	// URL: https://frontendapi.twitcasting.tv/users/[ID]/latest-movie

	movieUrl := fmt.Sprintf("https://frontendapi.twitcasting.tv/users/%s/latest-movie", screenID)
	movieReq, _ := http.NewRequest("GET", movieUrl, nil)
	movieReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	if movieResp, err := client.Do(movieReq); err == nil {
		defer movieResp.Body.Close()
		if movieResp.StatusCode == 200 {
			var movieRespData struct {
				Movie struct {
					ID     string `json:"id"`
					Title  string `json:"title"`
					IsLive bool   `json:"is_on_live"`
				} `json:"movie"`
			}
			if err := json.NewDecoder(movieResp.Body).Decode(&movieRespData); err == nil {
				meta.IsLive = movieRespData.Movie.IsLive
				if meta.IsLive {
					meta.LiveTitle = movieRespData.Movie.Title
				}
			}
		}
	}

	return meta, nil
}

// FetchUserMetadata refreshes stable profile fields without making a live-status request.
func FetchUserMetadata(screenID string) (*StreamerMetadata, error) {
	endpoint := fmt.Sprintf("https://frontendapi.twitcasting.tv/users/%s", screenID)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", browserUserAgent)
		resp, err := newHTTPClient(10 * time.Second).Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var apiResp struct {
			User struct {
				ScreenID string `json:"screenName"`
				Name     string `json:"name"`
				Image    string `json:"image"`
			} `json:"user"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&apiResp)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("user metadata API returned status %d", resp.StatusCode)
			continue
		}
		if decodeErr != nil {
			lastErr = decodeErr
			continue
		}
		avatar := normalizeAvatarURL(apiResp.User.Image)
		if strings.TrimSpace(apiResp.User.Name) == "" && avatar == "" {
			lastErr = fmt.Errorf("user metadata response is empty")
			continue
		}
		return &StreamerMetadata{ScreenID: apiResp.User.ScreenID, Nickname: apiResp.User.Name, Avatar: avatar}, nil
	}
	return nil, fmt.Errorf("fetch user metadata failed: %w", lastErr)
}

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124 Safari/537.36"

func normalizeAvatarURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

// FetchAvatar downloads an avatar through the same proxy path used by metadata requests.
func FetchAvatar(rawURL string) ([]byte, string, error) {
	avatarURL := normalizeAvatarURL(rawURL)
	if avatarURL == "" {
		return nil, "", fmt.Errorf("avatar URL is empty")
	}
	req, err := http.NewRequest(http.MethodGet, avatarURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	resp, err := newHTTPClient(12 * time.Second).Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("avatar request returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || len(data) > 2*1024*1024 {
		return nil, "", fmt.Errorf("avatar response size is invalid")
	}
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") || bytes.HasPrefix(data, []byte("<")) {
		return nil, "", fmt.Errorf("avatar response is not an image")
	}
	return data, mimeType, nil
}
