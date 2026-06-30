package checker

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/cookies"

	"github.com/jmoiron/jsonq"
)

// StreamInfo holds basic information about a live stream.
type StreamInfo struct {
	IsLive     bool
	StreamerID string
	Title      string
	StreamURL  string
	MovieID    string
	Created    int64
}

// ProtectedLiveError reports a live that exists but cannot be watched by the
// current account/auth context.
type ProtectedLiveError struct {
	ScreenID string
	Title    string
	LiveKey  string
}

func (e *ProtectedLiveError) Error() string {
	return "current live is protected (is_protected=true)"
}

// AsProtectedLiveError extracts protected-live metadata from an error.
func AsProtectedLiveError(err error) (*ProtectedLiveError, bool) {
	var protected *ProtectedLiveError
	if errors.As(err, &protected) {
		return protected, true
	}
	return nil, false
}

// LiveSessionKey identifies one live session well enough to suppress repeated
// attempts until that session ends.
func LiveSessionKey(info *StreamInfo) string {
	if info == nil {
		return ""
	}
	if movieID := strings.TrimSpace(info.MovieID); movieID != "" {
		return "movie:" + movieID
	}
	if info.Created > 0 {
		return fmt.Sprintf("created:%d", info.Created)
	}
	return "current-live"
}

// SameLiveSessionForSuppression returns true when a previously restricted live
// should still suppress recording for the current candidate. Some legacy Cookie
// responses do not expose a stable movie_id and may change created/current-live
// style keys between checks, so only two different explicit movie IDs are
// treated as a confirmed new session.
func SameLiveSessionForSuppression(restrictedKey, currentKey string) bool {
	restrictedKey = strings.TrimSpace(restrictedKey)
	currentKey = strings.TrimSpace(currentKey)
	if restrictedKey == "" {
		return false
	}
	if currentKey == "" || restrictedKey == currentKey {
		return true
	}
	if strings.HasPrefix(restrictedKey, "movie:") && strings.HasPrefix(currentKey, "movie:") {
		return false
	}
	return true
}

type currentLiveResp struct {
	Movie struct {
		ID          any    `json:"id"`
		IsLive      bool   `json:"is_live"`
		Title       string `json:"title"`
		HLSURL      string `json:"hls_url"`
		IsProtected bool   `json:"is_protected"`
		Created     int64  `json:"created"`
	} `json:"movie"`
}

type AuthOptions struct {
	Mode          string
	AccessToken   string
	CookieEnabled bool
	CookieFile    string
}

var proxyMu sync.RWMutex
var proxyURL string

// SetProxyURL configures an optional HTTP proxy for TwitCasting status checks.
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

func isTimeoutError(err error) bool {
	type timeout interface {
		Timeout() bool
	}
	if t, ok := err.(timeout); ok {
		return t.Timeout()
	}
	return false
}

// CheckStreamStatus checks if a streamer is live and returns stream info.
func CheckStreamStatus(screenID, accessToken string) (*StreamInfo, error) {
	return CheckStreamStatusWithAuth(screenID, AuthOptions{AccessToken: accessToken})
}

func CheckStreamStatusWithAuth(screenID string, auth AuthOptions) (*StreamInfo, error) {
	mode := strings.ToLower(strings.TrimSpace(auth.Mode))
	if mode == "" {
		mode = "auto"
	}
	token := strings.TrimSpace(auth.AccessToken)
	useCookie := auth.CookieEnabled && (mode == "auto" || mode == "cookie")

	if token != "" && mode != "cookie" {
		if info, handled, err := checkViaOAuthCurrentLive(screenID, token); handled {
			return info, err
		}
	}

	return checkViaLegacyStreamServer(screenID, token, cookieHeader(auth.CookieFile, useCookie))
}

func checkViaOAuthCurrentLive(screenID, accessToken string) (*StreamInfo, bool, error) {
	endpoint := fmt.Sprintf("https://apiv2.twitcasting.tv/users/%s/current_live", url.PathEscape(screenID))
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, true, fmt.Errorf("oauth request build failed: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Api-Version", "2.0")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := newHTTPClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		// transient network/API issue -> fallback to legacy chain
		return nil, false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &StreamInfo{IsLive: false, StreamerID: screenID}, true, nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		// Some current_live requests can be rejected per-user even when
		// verify_credentials succeeds. Fall back to the web status chain so a
		// per-streamer OAuth refusal does not stop monitoring entirely.
		return nil, false, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, true, fmt.Errorf("oauth rate limited (status 429)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// other API errors -> fallback to legacy chain
		return nil, false, nil
	}

	var payload currentLiveResp
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, false, nil
	}

	if !payload.Movie.IsLive {
		return &StreamInfo{IsLive: false, StreamerID: screenID}, true, nil
	}
	if payload.Movie.IsProtected {
		liveKey := LiveSessionKey(&StreamInfo{
			MovieID: stringifyID(payload.Movie.ID),
			Created: payload.Movie.Created,
		})
		return nil, true, &ProtectedLiveError{
			ScreenID: screenID,
			Title:    strings.TrimSpace(payload.Movie.Title),
			LiveKey:  liveKey,
		}
	}

	title := strings.TrimSpace(payload.Movie.Title)
	if title == "" {
		title = "untitled"
	}
	streamURL := strings.TrimSpace(payload.Movie.HLSURL)
	if streamURL == "" {
		return nil, true, fmt.Errorf("oauth current_live returned empty hls_url")
	}
	if !IsRecordableLiveURL(streamURL) {
		return nil, true, fmt.Errorf("oauth current_live returned non-live stream URL")
	}

	return &StreamInfo{
		IsLive:     true,
		StreamerID: screenID,
		Title:      title,
		StreamURL:  streamURL,
		MovieID:    stringifyID(payload.Movie.ID),
		Created:    payload.Movie.Created,
	}, true, nil
}

func checkViaLegacyStreamServer(screenID, accessToken, cookie string) (*StreamInfo, error) {
	apiEndpoint := "https://twitcasting.tv/streamserver.php"
	u, _ := url.Parse(apiEndpoint)
	q := u.Query()
	q.Set("target", screenID)
	q.Set("mode", "client")
	q.Set("player", "pc_web")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Referer", fmt.Sprintf("https://twitcasting.tv/%s", screenID))
	if strings.TrimSpace(cookie) != "" {
		req.Header.Set("Cookie", cookie)
	}

	timeout := 10 * time.Second
	if strings.TrimSpace(cookie) != "" {
		timeout = 20 * time.Second
	}
	client := newHTTPClient(timeout)
	resp, err := client.Do(req)
	if err != nil {
		if isTimeoutError(err) {
			time.Sleep(1200 * time.Millisecond)
			resp, err = client.Do(req.Clone(req.Context()))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned status: %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode json failed: %w", err)
	}

	jq := jsonq.NewQuery(data)
	isLive, err := jq.Bool("movie", "live")
	if err != nil || !isLive {
		return &StreamInfo{IsLive: false, StreamerID: screenID}, nil
	}

	title := FetchLiveTitleFromScraping(screenID)
	if strings.TrimSpace(accessToken) != "" {
		if apiTitle, ok := fetchCurrentLiveTitleViaOAuth(screenID, accessToken); ok {
			title = apiTitle
		}
	}
	if title == "" {
		title = "untitled"
	}

	streamURL := extractPreferredHLSStream(data)
	if streamURL == "" {
		streamURL = extractStreamURL(data)
	}
	if !IsRecordableLiveURL(streamURL) {
		return nil, fmt.Errorf("live detected but no recordable HLS stream URL was found")
	}
	movieID := ""
	if id, err := jq.String("movie", "id"); err == nil {
		movieID = strings.TrimSpace(id)
	}
	created := int64(0)
	if value, err := jq.Int("movie", "created"); err == nil {
		created = int64(value)
	}

	return &StreamInfo{
		IsLive:     true,
		StreamerID: screenID,
		Title:      title,
		StreamURL:  streamURL,
		MovieID:    movieID,
		Created:    created,
	}, nil
}

func cookieHeader(cookieFile string, enabled bool) string {
	if !enabled {
		return ""
	}
	return cookies.BuildHeader(cookieFile)
}

func extractPreferredHLSStream(data map[string]interface{}) string {
	tcHLS, ok := data["tc-hls"].(map[string]interface{})
	if !ok {
		return ""
	}
	streams, ok := tcHLS["streams"].(map[string]interface{})
	if !ok {
		return ""
	}

	for _, quality := range []string{"high", "medium", "low"} {
		if u, ok := stringFromStreamValue(streams[quality]); ok {
			return u
		}
	}
	for _, value := range streams {
		if u, ok := stringFromStreamValue(value); ok {
			return u
		}
	}
	return ""
}

func stringFromStreamValue(value interface{}) (string, bool) {
	switch v := value.(type) {
	case string:
		u := strings.TrimSpace(v)
		if isHTTPStreamURL(u) {
			return u, true
		}
	case map[string]interface{}:
		for _, key := range []string{"url", "src", "hls_url"} {
			if raw, ok := v[key].(string); ok {
				u := strings.TrimSpace(raw)
				if isHTTPStreamURL(u) {
					return u, true
				}
			}
		}
	}
	return "", false
}

func isHTTPStreamURL(u string) bool {
	lowerURL := strings.ToLower(strings.TrimSpace(u))
	return strings.HasPrefix(lowerURL, "http://") || strings.HasPrefix(lowerURL, "https://")
}

// IsRecordableLiveURL rejects archive/movie URLs and weak fallbacks before ffmpeg is started.
func IsRecordableLiveURL(rawURL string) bool {
	lowerURL := strings.ToLower(strings.TrimSpace(rawURL))
	if lowerURL == "" {
		return false
	}
	if !strings.HasPrefix(lowerURL, "http://") && !strings.HasPrefix(lowerURL, "https://") {
		return false
	}
	if strings.Contains(lowerURL, "/movie/") ||
		strings.Contains(lowerURL, "/archive/") ||
		strings.Contains(lowerURL, "metastream") ||
		strings.Contains(lowerURL, "recorded") {
		return false
	}
	return strings.Contains(lowerURL, ".m3u8")
}

func fetchCurrentLiveTitleViaOAuth(screenID, accessToken string) (string, bool) {
	endpoint := fmt.Sprintf("https://apiv2.twitcasting.tv/users/%s/current_live", url.PathEscape(screenID))
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("X-Api-Version", "2.0")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := newHTTPClient(8 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", false
	}

	jq := jsonq.NewQuery(payload)
	title, err := jq.String("movie", "title")
	if err == nil && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title), true
	}
	return "", false
}

func extractStreamURL(data map[string]interface{}) string {
	type candidate struct {
		URL   string
		Score int
	}

	candidates := make([]candidate, 0, 16)

	var walk func(node interface{}, path string)
	walk = func(node interface{}, path string) {
		switch v := node.(type) {
		case map[string]interface{}:
			for k, vv := range v {
				nextPath := k
				if path != "" {
					nextPath = path + "." + k
				}
				walk(vv, nextPath)
			}
		case []interface{}:
			for _, vv := range v {
				walk(vv, path)
			}
		case string:
			u := strings.TrimSpace(v)
			if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				return
			}
			lowerURL := strings.ToLower(u)
			if strings.HasPrefix(lowerURL, "ws://") || strings.HasPrefix(lowerURL, "wss://") {
				return
			}

			score := 0
			lowerPath := strings.ToLower(path)

			if strings.Contains(lowerPath, "tc-hls") {
				score += 50
			}
			if strings.Contains(lowerPath, "stream") {
				score += 20
			}
			if strings.Contains(lowerURL, "m3u8") {
				score += 40
			}
			if strings.Contains(lowerURL, "metastream") {
				score += 60
			}
			if strings.Contains(lowerURL, "playlist") {
				score += 20
			}
			if strings.Contains(lowerURL, "llfmp4") {
				score -= 20
			}
			if strings.Contains(lowerURL, ".flv") || strings.Contains(lowerURL, "rtmp") {
				score += 10
			}

			candidates = append(candidates, candidate{URL: u, Score: score})
		}
	}

	walk(data, "")
	if len(candidates) == 0 {
		return ""
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	return candidates[0].URL
}

// FetchLiveTitleFromScraping fetches the title strictly by scraping the HTML page.
func FetchLiveTitleFromScraping(screenID string) string {
	scrapeURL := fmt.Sprintf("https://twitcasting.tv/%s", screenID)
	client := newHTTPClient(5 * time.Second)
	scrapeReq, _ := http.NewRequest("GET", scrapeURL, nil)
	scrapeReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	if resp, err := client.Do(scrapeReq); err == nil {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyString := string(bodyBytes)

		if strings.Contains(bodyString, `property="og:title"`) {
			parts := strings.Split(bodyString, `property="og:title"`)
			if len(parts) > 1 {
				contentParts := strings.Split(parts[1], `content="`)
				if len(contentParts) > 1 {
					title := strings.Split(contentParts[1], `"`)[0]
					return cleanTitle(title)
				}
			}
		}

		if strings.Contains(bodyString, "<title>") {
			parts := strings.Split(bodyString, "<title>")
			if len(parts) > 1 {
				title := strings.Split(parts[1], "</title>")[0]
				title = strings.TrimSpace(title)
				return cleanTitle(title)
			}
		}
	}

	return ""
}

func cleanTitle(title string) string {
	title = html.UnescapeString(title)
	if idx := strings.LastIndex(title, " - TwitCasting"); idx != -1 {
		title = title[:idx]
	}
	return strings.TrimSpace(title)
}

func stringifyID(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}
