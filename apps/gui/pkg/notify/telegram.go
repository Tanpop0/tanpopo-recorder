package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
)

// SendTelegram sends one Telegram bot message. It is intentionally small so
// scheduler events can use it without pulling notification policy into recorder.
func SendTelegram(ctx context.Context, cfg config.TelegramConfig, proxyRawURL, text string) error {
	if !cfg.Enabled {
		return nil
	}
	token := strings.TrimSpace(cfg.BotToken)
	chatID := strings.TrimSpace(cfg.ChatID)
	text = strings.TrimSpace(text)
	if token == "" || chatID == "" || text == "" {
		return nil
	}

	endpoint := "https://api.telegram.org/bot" + token + "/sendMessage"
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")

	client := &http.Client{Timeout: 12 * time.Second}
	if strings.TrimSpace(proxyRawURL) != "" {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		if proxyURL, err := url.Parse(strings.TrimSpace(proxyRawURL)); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
			client.Transport = transport
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("telegram api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
