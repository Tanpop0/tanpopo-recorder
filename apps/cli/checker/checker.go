package checker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jmoiron/jsonq"
)

// StreamInfo holds basic information about a live stream
type StreamInfo struct {
	IsLive     bool
	StreamerID string
}

// CheckStreamStatus checks if a streamer is live and returns WebSocket URL
func CheckStreamStatus(screenID string) (*StreamInfo, error) {
	apiEndpoint := "https://twitcasting.tv/streamserver.php"
	u, _ := url.Parse(apiEndpoint)
	q := u.Query()
	q.Set("target", screenID)
	q.Set("mode", "client")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Important Headers from reference repo
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Referer", fmt.Sprintf("https://twitcasting.tv/%s", screenID))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
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

	// Check if live
	isLive, err := jq.Bool("movie", "live")
	if err != nil || !isLive {
		return &StreamInfo{IsLive: false}, nil
	}

	return &StreamInfo{
		IsLive:     true,
		StreamerID: screenID,
	}, nil
}
