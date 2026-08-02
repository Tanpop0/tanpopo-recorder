package paths

import (
	"path/filepath"
	"strings"
)

const (
	ConfigFileName  = "config.yaml"
	CookiesFileName = "cookies.txt"
	HistoryFileName = "history.json"
	LogsDirName     = "logs"
	AvatarCacheName = "avatar-cache"
	CLILogFileName  = "cli.log"
)

func DefaultConfigPath() string {
	return ConfigFileName
}

func DefaultCookiesPath() string {
	return CookiesFileName
}

func ConfigDir(configPath string) string {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return "."
	}
	dir := filepath.Dir(configPath)
	if dir == "" {
		return "."
	}
	return dir
}

func HistoryPath(configPath string) string {
	return filepath.Join(ConfigDir(configPath), HistoryFileName)
}

func AvatarCacheDir(configPath string) string {
	return filepath.Join(ConfigDir(configPath), AvatarCacheName)
}

func LogsDir(outputDir string) string {
	outputDir = strings.TrimSpace(outputDir)
	if outputDir == "" || outputDir == "." {
		return LogsDirName
	}
	return filepath.Join(outputDir, LogsDirName)
}

func CLILogPath(outputDir string) string {
	return filepath.Join(LogsDir(outputDir), CLILogFileName)
}
