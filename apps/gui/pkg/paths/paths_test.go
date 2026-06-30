package paths

import (
	"path/filepath"
	"testing"
)

func TestHistoryPathFollowsConfigDirectory(t *testing.T) {
	got := HistoryPath(filepath.Join("app", ConfigFileName))
	want := filepath.Join("app", HistoryFileName)
	if got != want {
		t.Fatalf("HistoryPath() = %q, want %q", got, want)
	}
}

func TestLogsDirUsesPortableDefault(t *testing.T) {
	if got := LogsDir(""); got != LogsDirName {
		t.Fatalf("LogsDir(empty) = %q, want %q", got, LogsDirName)
	}
	if got := LogsDir("."); got != LogsDirName {
		t.Fatalf("LogsDir(.) = %q, want %q", got, LogsDirName)
	}
}
