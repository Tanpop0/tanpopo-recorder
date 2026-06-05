package comments

import (
	"testing"
	"time"
)

func TestFormatTextLine(t *testing.T) {
	got := formatTextLine(TimelineComment{
		T:        12.4,
		Name:     "Alice",
		ScreenID: "alice_id",
		Text:     "hello world",
	}, "")
	want := "[00:00:12] Alice (@alice_id): hello world"
	if got != want {
		t.Fatalf("formatTextLine() = %q, want %q", got, want)
	}
}

func TestFormatTextLineCustomTemplate(t *testing.T) {
	got := formatTextLine(TimelineComment{
		ID:      "123",
		T:       65.2,
		Created: 1700000000,
		UserID:  "u1",
		Name:    "Alice",
		Text:    "hello",
	}, "{offset}|{name}|{screen_id}|{user_id}|{id}|{message}")
	want := "00:01:05|Alice||u1|123|hello"
	if got != want {
		t.Fatalf("formatTextLine(custom) = %q, want %q", got, want)
	}
}

func TestFilterNewCommentsSkipsOldAndDuplicate(t *testing.T) {
	start := time.Now().Unix()
	seen := map[string]bool{"1": true}
	got := filterNewComments([]Comment{
		{ID: "1", Created: start + 1},
		{ID: "2", Created: start - 10},
		{ID: "3", Created: start + 2, Message: "ok"},
	}, seen, start-3)
	if len(got) != 1 || got[0].ID != "3" {
		t.Fatalf("filterNewComments() = %#v, want only comment 3", got)
	}
}
