package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/paths"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/recorder"
)

func (a *App) CheckRecordingTools() recorder.ToolStatus {
	a.mu.RLock()
	var opts recorder.RecordOptions
	if a.config != nil {
		opts.FFmpegPath = a.config.Recording.FFmpegPath
		opts.FFprobePath = a.config.Recording.FFprobePath
	}
	a.mu.RUnlock()
	return recorder.CheckToolchain(opts)
}

func (a *App) CheckRecordingToolsWithPaths(ffmpegPath, ffprobePath string) recorder.ToolStatus {
	return recorder.CheckToolchain(recorder.RecordOptions{
		FFmpegPath:  strings.TrimSpace(ffmpegPath),
		FFprobePath: strings.TrimSpace(ffprobePath),
	})
}

func (a *App) RunHealthCheck() HealthCheckReport {
	a.mu.RLock()
	cfg := a.config
	a.mu.RUnlock()
	return a.runHealthCheckForConfig(cfg)
}

func (a *App) runHealthCheckForConfig(cfg *config.Config) HealthCheckReport {
	report := HealthCheckReport{OK: true, Items: make([]HealthCheckItem, 0, 8)}
	add := func(name, status, message string) {
		if status == "error" {
			report.OK = false
		}
		report.Items = append(report.Items, HealthCheckItem{Name: name, Status: status, Message: message})
	}

	if cfg == nil {
		add("配置", "error", "配置尚未加载")
		return report
	}
	add("配置文件", "ok", paths.DefaultConfigPath())
	add("历史记录", "ok", paths.HistoryPath(paths.DefaultConfigPath()))

	toolStatus := recorder.CheckToolchain(recorder.RecordOptions{
		FFmpegPath:  cfg.Recording.FFmpegPath,
		FFprobePath: cfg.Recording.FFprobePath,
	})
	if toolStatus.FFmpegOK {
		add("FFmpeg", "ok", toolStatus.FFmpegPath)
	} else {
		add("FFmpeg", "error", toolStatus.Message)
	}
	if toolStatus.FFprobeOK {
		add("FFprobe", "ok", toolStatus.FFprobePath)
	} else {
		add("FFprobe", "warn", "FFprobe is not available; recording can continue, but duration probing may be less accurate")
	}

	outputDir := cfg.OutputDirectory
	if strings.TrimSpace(outputDir) == "" {
		outputDir = "."
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		add("输出目录", "error", err.Error())
	} else {
		testPath := filepath.Join(outputDir, ".twitcasting-write-test")
		if err := os.WriteFile(testPath, []byte("ok"), 0644); err != nil {
			add("输出目录", "error", "不可写: "+err.Error())
		} else {
			_ = os.Remove(testPath)
			add("输出目录", "ok", outputDir)
		}
	}
	logDir := paths.LogsDir(outputDir)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		add("日志目录", "warn", err.Error())
	} else {
		add("日志目录", "ok", logDir)
	}

	if cfg.Recording.WorkerEnabled {
		if path, err := findWorkerExecutable(cfg.Recording.WorkerPath); err != nil {
			add("Worker", "error", err.Error())
		} else {
			add("Worker", "ok", path)
		}
	} else {
		add("Worker", "warn", "未启用 worker 进程隔离")
	}

	if cfg.Cookies.Enabled {
		if _, err := os.Stat(cfg.Cookies.FilePath); err != nil {
			add("Cookie", "warn", "已启用但文件不可读: "+err.Error())
		} else {
			add("Cookie", "ok", cfg.Cookies.FilePath)
		}
	} else {
		add("Cookie", "warn", "未启用 cookies.txt，会员或受限直播可能失败")
	}

	if strings.TrimSpace(cfg.OAuth.AccessToken) != "" {
		add("OAuth", "ok", "Access Token configured")
	} else {
		add("OAuth", "warn", "Access Token is empty; official API checks and comment capture may be limited")
	}

	if cfg.Proxy.Enabled {
		u, err := url.Parse(cfg.Proxy.URL)
		if err != nil {
			add("代理", "error", "代理地址格式不正确: "+err.Error())
		} else if u.Scheme == "" || u.Host == "" {
			add("代理", "error", "代理地址需要包含协议和主机，例如 http://127.0.0.1:7890")
		} else {
			add("代理", "ok", cfg.Proxy.URL)
		}
	} else {
		add("Proxy", "ok", "Proxy disabled")
	}

	return report
}

func (a *App) RunHealthCheckWithSettings(payload SettingsPayload) HealthCheckReport {
	a.mu.RLock()
	original := a.config
	a.mu.RUnlock()

	cfg := &config.Config{
		OutputDirectory: payload.OutputDirectory,
		AuthMode:        payload.AuthMode,
		OAuth:           payload.OAuth,
		Cookies:         payload.Cookies,
		Recording:       payload.Recording,
		Proxy:           payload.Proxy,
		Notifications:   payload.Notifications,
	}
	if original != nil {
		cfg.Streamers = append([]config.StreamerConfig(nil), original.Streamers...)
	}
	return a.runHealthCheckForConfig(cfg)
}

func findWorkerExecutable(configuredPath string) (string, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath != "" {
		if _, err := os.Stat(configuredPath); err == nil {
			return configuredPath, nil
		}
		return "", fmt.Errorf("worker executable not found: %s", configuredPath)
	}

	candidates := make([]string, 0, 4)
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, exe)
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "recorder-worker.exe"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "recorder-worker.exe"))
		candidates = append(candidates, filepath.Join(cwd, "cmd", "recorder-worker", "recorder-worker.exe"))
	}
	if path, err := exec.LookPath("recorder-worker.exe"); err == nil {
		candidates = append(candidates, path)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("embedded worker or recorder-worker.exe not found")
}
