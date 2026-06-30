package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/paths"
)

func (a *App) OpenFolder(filePath string) {
	if filePath == "" {
		return
	}
	dir := filepath.Dir(filePath)

	var cmd *exec.Cmd
	if stdruntime.GOOS == "windows" {
		cmd = exec.Command("explorer", dir)
	} else if stdruntime.GOOS == "darwin" {
		cmd = exec.Command("open", dir)
	} else {
		cmd = exec.Command("xdg-open", dir)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error opening folder: %v", err)
	}
}

func (a *App) OpenFile(filePath string) string {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "file path is empty"
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Sprintf("file not found: %v", err)
	}
	if info.IsDir() {
		return "refuse to open a directory as file"
	}

	var cmd *exec.Cmd
	if stdruntime.GOOS == "windows" {
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", filePath)
	} else if stdruntime.GOOS == "darwin" {
		cmd = exec.Command("open", filePath)
	} else {
		cmd = exec.Command("xdg-open", filePath)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("open file failed: %v", err)
	}
	return ""
}

func (a *App) OpenStreamerLogFolder(screenID string) {
	screenID = normalizeID(screenID)
	if screenID == "" {
		return
	}
	logDir := paths.LogsDir("")
	a.mu.RLock()
	if a.config != nil && strings.TrimSpace(a.config.OutputDirectory) != "" {
		logDir = paths.LogsDir(a.config.OutputDirectory)
	}
	a.mu.RUnlock()
	_ = os.MkdirAll(logDir, 0755)

	var cmd *exec.Cmd
	if stdruntime.GOOS == "windows" {
		cmd = exec.Command("explorer", logDir)
	} else if stdruntime.GOOS == "darwin" {
		cmd = exec.Command("open", logDir)
	} else {
		cmd = exec.Command("xdg-open", logDir)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Error opening log folder: %v", err)
	}
}
