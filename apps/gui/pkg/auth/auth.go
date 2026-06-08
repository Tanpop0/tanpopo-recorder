package auth

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

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
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

func BuildAuthorizeURL(clientID, redirectURI, state string) string {
	v := url.Values{}
	v.Set("client_id", strings.TrimSpace(clientID))
	v.Set("response_type", "code")
	if strings.TrimSpace(redirectURI) != "" {
		v.Set("redirect_uri", strings.TrimSpace(redirectURI))
	}
	if strings.TrimSpace(state) != "" {
		v.Set("state", strings.TrimSpace(state))
	}
	return "https://apiv2.twitcasting.tv/oauth2/authorize?" + v.Encode()
}

func ExchangeCode(clientID, clientSecret, redirectURI, code string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", strings.TrimSpace(clientID))
	form.Set("client_secret", strings.TrimSpace(clientSecret))
	form.Set("code", strings.TrimSpace(code))
	if strings.TrimSpace(redirectURI) != "" {
		form.Set("redirect_uri", strings.TrimSpace(redirectURI))
	}

	req, err := http.NewRequest("POST", "https://apiv2.twitcasting.tv/oauth2/access_token", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Version", "2.0")

	client := newHTTPClient(15 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.Join(strings.Fields(string(body)), " ")
		if message != "" {
			return nil, fmt.Errorf("token endpoint status: %d, response: %s", resp.StatusCode, message)
		}
		return nil, fmt.Errorf("token endpoint status: %d", resp.StatusCode)
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	if strings.TrimSpace(tr.AccessToken) == "" {
		return nil, fmt.Errorf("empty access_token from token endpoint")
	}
	if strings.TrimSpace(tr.TokenType) == "" {
		tr.TokenType = "bearer"
	}
	return &tr, nil
}

func VerifyAccessToken(accessToken string) error {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return fmt.Errorf("access token is empty")
	}

	req, err := http.NewRequest("GET", "https://apiv2.twitcasting.tv/verify_credentials", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Api-Version", "2.0")

	client := newHTTPClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("verify_credentials status: %d", resp.StatusCode)
	}
	return nil
}
