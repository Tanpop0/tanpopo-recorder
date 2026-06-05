package metadata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type StreamerMetadata struct {
	ScreenID string `json:"screen_id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	IsLive   bool   `json:"is_live"`
	LiveTitle string `json:"live_title"`
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
    
    url := fmt.Sprintf("https://frontendapi.twitcasting.tv/users/%s", screenID)
    client := &http.Client{Timeout: 10 * time.Second}
    
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }
    
    // Set headers to look like a browser
    req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
    
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("API returned status: %d", resp.StatusCode)
    }
    
    // Define a partial struct for the API response
    var apiResp struct {
        User struct {
            ID       string `json:"id"`
            ScreenID string `json:"screen_id"`
            Name     string `json:"name"`
            Image    string `json:"image"`
            IsLive   bool   `json:"is_live"` // This field might not exist or be named differently, need to verify
        } `json:"user"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
        return nil, err
    }
    
    meta := &StreamerMetadata{
        ScreenID: apiResp.User.ScreenID,
        Nickname: apiResp.User.Name,
        Avatar:   apiResp.User.Image,
    }
    
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
                    ID        string `json:"id"`
                    Title     string `json:"title"`
                    IsLive    bool   `json:"is_on_live"`
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
