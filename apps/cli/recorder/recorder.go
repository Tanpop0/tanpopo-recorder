package recorder

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RecordStreamStreamlink executes streamlink to record the stream
func RecordStreamStreamlink(screenID, outputDir string) error {
	// 1. Check if streamlink is installed
	streamlinkPath, err := exec.LookPath("streamlink")
	if err != nil {
		// Fallback: Check common Windows installation paths
		commonPaths := []string{
			`C:\Program Files (x86)\Streamlink\bin\streamlink.exe`,
			`C:\Program Files\Streamlink\bin\streamlink.exe`,
		}
		found := false
		for _, path := range commonPaths {
			if _, err := os.Stat(path); err == nil {
				streamlinkPath = path
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("streamlink not found in PATH or common directories. Please install Streamlink: https://github.com/streamlink/streamlink/releases")
		}
	}

	// 2. Prepare output filename
	// Sanitize screenID for filename (Windows doesn't allow colon :)
	safeScreenID := strings.ReplaceAll(screenID, ":", "_")
	filename := fmt.Sprintf("%s-%s.ts", safeScreenID, time.Now().Format("20060102-150405"))
	outputPath := fmt.Sprintf("%s/%s", outputDir, filename)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}

	streamURL := fmt.Sprintf("https://twitcasting.tv/%s", screenID)

	// 3. Construct command
	// streamlink "url" best -o "output.ts" --force
	cmd := exec.Command(streamlinkPath, streamURL, "best", "-o", outputPath, "--force")

	// 4. Pipe stdout/stderr to console so user can see progress
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Starting Streamlink for %s...\nCommand: %s\n", screenID, cmd.String())

	// 5. Run and wait
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("streamlink exited with error: %w", err)
	}

	fmt.Printf("Streamlink finished recording %s\n", screenID)
	return nil
}
