package cookies

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

type Cookie struct {
	Domain string
	Path   string
	Name   string
	Value  string
}

// BuildHeader converts a Netscape cookies.txt file into an HTTP Cookie header.
func BuildHeader(cookieFile string) string {
	parsed := parseTwitCastingCookies(cookieFile)
	if len(parsed) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(parsed))
	for _, item := range parsed {
		pairs = append(pairs, item.Name+"="+item.Value)
	}
	return strings.Join(pairs, "; ")
}

// BuildFFmpegCookieJar converts cookies.txt into FFmpeg's native -cookies format.
func BuildFFmpegCookieJar(cookieFile string) string {
	parsed := parseTwitCastingCookies(cookieFile)
	if len(parsed) == 0 {
		return ""
	}
	lines := make([]string, 0, len(parsed))
	for _, item := range parsed {
		lines = append(lines, fmt.Sprintf("%s=%s; path=%s; domain=%s;", item.Name, item.Value, item.Path, item.Domain))
	}
	return strings.Join(lines, "\n")
}

func parseTwitCastingCookies(cookieFile string) []Cookie {
	cookieFile = strings.TrimSpace(cookieFile)
	if cookieFile == "" {
		return nil
	}
	if _, err := os.Stat(cookieFile); err != nil {
		return nil
	}

	f, err := os.Open(cookieFile)
	if err != nil {
		return nil
	}
	defer f.Close()

	now := time.Now().Unix()
	scanner := bufio.NewScanner(f)
	parsed := make([]Cookie, 0, 32)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}
		domain := strings.ToLower(parts[0])
		if !strings.Contains(domain, "twitcasting.tv") {
			continue
		}
		path := strings.TrimSpace(parts[2])
		if path == "" {
			path = "/"
		}
		expiryRaw := strings.TrimSpace(parts[4])
		name := strings.TrimSpace(parts[5])
		value := strings.TrimSpace(parts[6])
		if name == "" {
			continue
		}
		if expiryRaw != "" && expiryRaw != "0" {
			var exp int64
			_, err := fmt.Sscan(expiryRaw, &exp)
			if err == nil && exp > 0 && exp < now {
				continue
			}
		}
		parsed = append(parsed, Cookie{Domain: domain, Path: path, Name: name, Value: value})
	}

	return parsed
}
