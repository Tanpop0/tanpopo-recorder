package history

import "testing"

func TestFailureInfo(t *testing.T) {
	tests := []struct {
		status string
		detail string
		code   string
	}{
		{"failed_auth", "", "auth"},
		{"failed_network", "", "network"},
		{"failed_stream", "Server returned 404 Not Found", "stream"},
		{"failed", "context deadline exceeded", "network"},
		{"completed", "", ""},
	}
	for _, tt := range tests {
		code, summary := FailureInfo(tt.status, tt.detail)
		if code != tt.code {
			t.Fatalf("FailureInfo(%q, %q) code = %q, want %q", tt.status, tt.detail, code, tt.code)
		}
		if tt.code != "" && summary == "" {
			t.Fatalf("FailureInfo(%q, %q) returned empty summary", tt.status, tt.detail)
		}
	}
}
