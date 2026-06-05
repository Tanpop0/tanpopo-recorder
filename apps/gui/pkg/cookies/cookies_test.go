package cookies

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildHeaderParsesTwitCastingCookies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.txt")
	exp := time.Now().Add(time.Hour).Unix()
	data := strings.Join([]string{
		"# Netscape HTTP Cookie File",
		".twitcasting.tv\tTRUE\t/\tTRUE\t" + strconvFormatInt(exp) + "\ttc_id\tabc",
		".example.com\tTRUE\t/\tTRUE\t" + strconvFormatInt(exp) + "\tignored\tvalue",
		".twitcasting.tv\tTRUE\t/\tTRUE\t1\texpired\told",
	}, "\n")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	got := BuildHeader(path)
	if !strings.Contains(got, "tc_id=abc") {
		t.Fatalf("BuildHeader() = %q, want tc_id", got)
	}
	if strings.Contains(got, "ignored=") || strings.Contains(got, "expired=") {
		t.Fatalf("BuildHeader() included non-twitcasting or expired cookies: %q", got)
	}
}

func TestBuildFFmpegCookieJarUsesDomainAndPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.txt")
	exp := time.Now().Add(time.Hour).Unix()
	data := strings.Join([]string{
		".twitcasting.tv\tTRUE\t/\tTRUE\t" + strconvFormatInt(exp) + "\ttc_ss\tsecret",
		".twitcasting.tv\tTRUE\t/member\tTRUE\t" + strconvFormatInt(exp) + "\ttc_id\tuser",
	}, "\n")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	got := BuildFFmpegCookieJar(path)
	if !strings.Contains(got, "tc_ss=secret; path=/; domain=.twitcasting.tv;") {
		t.Fatalf("BuildFFmpegCookieJar() = %q, want tc_ss line", got)
	}
	if !strings.Contains(got, "tc_id=user; path=/member; domain=.twitcasting.tv;") {
		t.Fatalf("BuildFFmpegCookieJar() = %q, want tc_id path line", got)
	}
}

func strconvFormatInt(value int64) string { return strconv.FormatInt(value, 10) }
