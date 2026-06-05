package comments

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultPollInterval = 8 * time.Second
const DefaultTextTemplate = "[{offset}] {display_name}: {message}"

var apiBaseURL = "https://apiv2.twitcasting.tv"

type Options struct {
	MovieID      string
	AccessToken  string
	TextPath     string
	JSONLPath    string
	StartTime    time.Time
	TextTemplate string
	ProxyURL     string
	Interval     time.Duration
	Logf         func(format string, args ...any)
}

type responsePayload struct {
	MovieID  string    `json:"movie_id"`
	AllCount int       `json:"all_count"`
	Comments []Comment `json:"comments"`
}

type Comment struct {
	ID       string `json:"id"`
	Message  string `json:"message"`
	FromUser User   `json:"from_user"`
	Created  int64  `json:"created"`
}

type User struct {
	ID       string `json:"id"`
	ScreenID string `json:"screen_id"`
	Name     string `json:"name"`
	Image    string `json:"image"`
}

type TimelineComment struct {
	ID       string  `json:"id"`
	T        float64 `json:"t"`
	Created  int64   `json:"created"`
	UserID   string  `json:"user_id"`
	ScreenID string  `json:"screen_id"`
	Name     string  `json:"name"`
	Image    string  `json:"image,omitempty"`
	Text     string  `json:"text"`
}

func Capture(ctx context.Context, opts Options) error {
	opts.MovieID = strings.TrimSpace(opts.MovieID)
	opts.AccessToken = strings.TrimSpace(opts.AccessToken)
	if opts.MovieID == "" {
		return fmt.Errorf("movie_id is empty")
	}
	if opts.AccessToken == "" {
		return fmt.Errorf("access_token is empty")
	}
	if opts.TextPath == "" && opts.JSONLPath == "" {
		return fmt.Errorf("comment output path is empty")
	}
	if opts.StartTime.IsZero() {
		opts.StartTime = time.Now()
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultPollInterval
	}

	if err := ensureParentDir(opts.TextPath); err != nil {
		return err
	}
	if err := ensureParentDir(opts.JSONLPath); err != nil {
		return err
	}

	var textWriter *bufio.Writer
	if opts.TextPath != "" {
		f, err := os.OpenFile(opts.TextPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		textWriter = bufio.NewWriter(f)
		defer textWriter.Flush()
		_, _ = fmt.Fprintf(textWriter, "TwitCasting Comments\r\nMovie ID: %s\r\nRecording Start: %s\r\n\r\n", opts.MovieID, opts.StartTime.Format(time.RFC3339))
		_ = textWriter.Flush()
	}

	var jsonWriter *bufio.Writer
	if opts.JSONLPath != "" {
		f, err := os.OpenFile(opts.JSONLPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		jsonWriter = bufio.NewWriter(f)
		defer jsonWriter.Flush()
	}

	client := newHTTPClient(10*time.Second, opts.ProxyURL)
	seen := make(map[string]bool)
	startUnix := opts.StartTime.Add(-3 * time.Second).Unix()
	var lastSliceID int64
	consecutiveErrors := 0

	for {
		comments, err := fetchComments(ctx, client, opts.MovieID, opts.AccessToken, lastSliceID)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			consecutiveErrors++
			if opts.Logf != nil && (consecutiveErrors == 1 || consecutiveErrors%6 == 0) {
				opts.Logf("Comment fetch failed: %v", err)
			}
		} else {
			consecutiveErrors = 0
			if maxID := maxNumericID(comments); maxID > lastSliceID {
				lastSliceID = maxID
			}
			filtered := filterNewComments(comments, seen, startUnix)
			sort.SliceStable(filtered, func(i, j int) bool {
				if filtered[i].Created == filtered[j].Created {
					return numericID(filtered[i].ID) < numericID(filtered[j].ID)
				}
				return filtered[i].Created < filtered[j].Created
			})
			for _, c := range filtered {
				item := toTimelineComment(c, opts.StartTime)
				if textWriter != nil {
					_, _ = fmt.Fprintln(textWriter, formatTextLine(item, opts.TextTemplate))
				}
				if jsonWriter != nil {
					if data, err := json.Marshal(item); err == nil {
						_, _ = jsonWriter.Write(data)
						_, _ = jsonWriter.WriteString("\n")
					}
				}
			}
			if len(filtered) > 0 {
				if textWriter != nil {
					_ = textWriter.Flush()
				}
				if jsonWriter != nil {
					_ = jsonWriter.Flush()
				}
			}
		}

		timer := time.NewTimer(opts.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func fetchComments(ctx context.Context, client *http.Client, movieID, accessToken string, sliceID int64) ([]Comment, error) {
	endpoint, err := url.Parse(strings.TrimRight(apiBaseURL, "/") + "/movies/" + url.PathEscape(movieID) + "/comments")
	if err != nil {
		return nil, err
	}
	q := endpoint.Query()
	q.Set("limit", "50")
	if sliceID > 0 {
		q.Set("slice_id", strconv.FormatInt(sliceID, 10))
	} else {
		q.Set("offset", "0")
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("X-Api-Version", "2.0")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("comments api returned status %d", resp.StatusCode)
	}

	var payload responsePayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Comments, nil
}

func filterNewComments(comments []Comment, seen map[string]bool, startUnix int64) []Comment {
	out := make([]Comment, 0, len(comments))
	for _, c := range comments {
		if strings.TrimSpace(c.ID) == "" {
			continue
		}
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		if c.Created > 0 && c.Created < startUnix {
			continue
		}
		out = append(out, c)
	}
	return out
}

func toTimelineComment(c Comment, startTime time.Time) TimelineComment {
	relative := 0.0
	if c.Created > 0 {
		relative = float64(c.Created) - float64(startTime.UnixNano())/float64(time.Second)
		if relative < 0 {
			relative = 0
		}
	}
	return TimelineComment{
		ID:       c.ID,
		T:        relative,
		Created:  c.Created,
		UserID:   c.FromUser.ID,
		ScreenID: c.FromUser.ScreenID,
		Name:     c.FromUser.Name,
		Image:    c.FromUser.Image,
		Text:     normalizeMessage(c.Message),
	}
}

func formatTextLine(c TimelineComment, template string) string {
	template = strings.TrimSpace(template)
	if template == "" {
		template = DefaultTextTemplate
	}
	replacements := map[string]string{
		"{id}":           c.ID,
		"{offset}":       formatRelativeTime(c.T),
		"{created}":      formatCreatedTime(c.Created),
		"{user_id}":      strings.TrimSpace(c.UserID),
		"{screen_id}":    strings.TrimSpace(c.ScreenID),
		"{name}":         strings.TrimSpace(c.Name),
		"{display_name}": displayName(c),
		"{message}":      c.Text,
	}
	out := template
	for key, value := range replacements {
		out = strings.ReplaceAll(out, key, value)
	}
	return out
}

func displayName(c TimelineComment) string {
	name := strings.TrimSpace(c.Name)
	screenID := strings.TrimSpace(c.ScreenID)
	switch {
	case name != "" && screenID != "":
		return fmt.Sprintf("%s (@%s)", name, screenID)
	case name == "" && screenID != "":
		return "@" + screenID
	case name == "":
		return "unknown"
	default:
		return name
	}
}

func formatCreatedTime(created int64) string {
	if created <= 0 {
		return ""
	}
	return time.Unix(created, 0).Format(time.RFC3339)
}

func formatRelativeTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	total := int64(seconds + 0.5)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func normalizeMessage(message string) string {
	message = strings.ReplaceAll(message, "\r", " ")
	message = strings.ReplaceAll(message, "\n", " ")
	return strings.Join(strings.Fields(message), " ")
}

func maxNumericID(comments []Comment) int64 {
	var maxID int64
	for _, c := range comments {
		if id := numericID(c.ID); id > maxID {
			maxID = id
		}
	}
	return maxID
}

func numericID(id string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	return n
}

func ensureParentDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return os.MkdirAll(filepath.Dir(path), 0755)
}

func newHTTPClient(timeout time.Duration, proxyRawURL string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.TrimSpace(proxyRawURL) != "" {
		if u, err := url.Parse(strings.TrimSpace(proxyRawURL)); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}
