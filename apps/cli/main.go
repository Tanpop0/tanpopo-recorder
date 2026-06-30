package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/auth"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/checker"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/history"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/notify"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/paths"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/recorder"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/scheduler"
)

const version = "0.2.0-cli"

type globalFlags struct {
	configPath string
	logFile    string
	verbose    bool
}

type cliNotifier struct {
	history  *history.HistoryManager
	logFile  string
	verbose  bool
	mu       sync.Mutex
	statuses map[string]cliStatus
	logs     []cliLog
}

type cliStatus struct {
	ScreenID string    `json:"screen_id"`
	Status   string    `json:"status"`
	Message  string    `json:"message"`
	Updated  time.Time `json:"updated"`
}

type cliLog struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runMonitor(args)
	}

	switch args[0] {
	case "monitor", "daemon":
		return runMonitor(args[1:])
	case "web", "server":
		return runWeb(args[1:])
	case "record", "once":
		return runRecord(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "auth":
		return runAuth(args[1:])
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func parseGlobalFlags(name string, args []string) (*globalFlags, []string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	flags := &globalFlags{}
	fs.StringVar(&flags.configPath, "config", paths.DefaultConfigPath(), "path to config yaml")
	fs.StringVar(&flags.logFile, "log-file", "", "path to CLI log file; default is <output>/logs/cli.log")
	fs.BoolVar(&flags.verbose, "verbose", false, "print verbose status logs")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	return flags, fs.Args(), nil
}

func runMonitor(args []string) error {
	flags, rest, err := parseGlobalFlags("monitor", args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("monitor does not accept positional arguments")
	}

	cfg, err := config.LoadConfig(flags.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	normalizeCLIConfig(cfg)
	notifier := newCLINotifier(cfg, flags)
	notifier.NotifyAppLog(fmt.Sprintf("twitrec %s starting monitor with %s", version, flags.configPath))

	manager := scheduler.NewManager(notifier, cfg)
	added := 0
	for _, streamer := range cfg.Streamers {
		if streamer.Disabled || strings.TrimSpace(streamer.ScreenID) == "" {
			continue
		}
		if strings.TrimSpace(streamer.Schedule) == "" {
			streamer.Schedule = defaultSchedule(cfg)
		}
		if err := manager.AddStreamer(streamer); err != nil {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] schedule failed: %v", streamer.ScreenID, err))
			continue
		}
		added++
		notifier.NotifyAppLog(fmt.Sprintf("[%s] scheduled: %s", streamer.ScreenID, streamer.Schedule))
		manager.SetMonitoring(streamer.ScreenID, true)
		if cfg.Recording.StartupStaggerSeconds > 0 {
			time.Sleep(time.Duration(cfg.Recording.StartupStaggerSeconds) * time.Second)
		}
	}
	if added == 0 {
		return errors.New("no enabled streamers in config")
	}

	manager.Start()
	fmt.Printf("TwitCasting CLI monitor started (%d streamers). Press Ctrl+C to stop.\n", added)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nStopping monitor...")
	notifier.NotifyAppLog("shutdown requested")
	manager.Stop()
	return nil
}

func runRecord(args []string) error {
	flags, rest, err := parseGlobalFlags("record", args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("usage: twitcasting-recorder record <screen_id> [--config config.yaml]")
	}
	screenID := strings.TrimSpace(rest[0])
	if screenID == "" {
		return errors.New("screen_id is empty")
	}

	cfg, err := config.LoadConfig(flags.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	normalizeCLIConfig(cfg)
	notifier := newCLINotifier(cfg, flags)
	checker.SetProxyURL(proxyURL(cfg))
	streamer := findStreamer(cfg, screenID)
	recAuth := authSettings(cfg, streamer)
	checkAuth := checker.AuthOptions{
		Mode:          recAuth.Mode,
		AccessToken:   recAuth.AccessToken,
		CookieEnabled: recAuth.CookieEnabled,
		CookieFile:    recAuth.CookieFile,
	}

	fmt.Printf("Checking %s...\n", screenID)
	info, err := checker.CheckStreamStatusWithAuth(screenID, checkAuth)
	if err != nil {
		return err
	}
	if info == nil || !info.IsLive {
		fmt.Printf("%s is offline.\n", screenID)
		return nil
	}
	if !checker.IsRecordableLiveURL(info.StreamURL) {
		return fmt.Errorf("live candidate is not recordable")
	}

	fmt.Println("Live candidate detected, confirming...")
	time.Sleep(5 * time.Second)
	confirmed, err := checker.CheckStreamStatusWithAuth(screenID, checkAuth)
	if err != nil {
		return fmt.Errorf("second live check failed: %w", err)
	}
	if confirmed == nil || !confirmed.IsLive || !checker.IsRecordableLiveURL(confirmed.StreamURL) {
		fmt.Printf("%s is no longer live after confirmation.\n", screenID)
		return nil
	}

	rc := recordOptions(cfg, streamer, confirmed.MovieID)
	sendCLITelegram(cfg, streamer, notifier, "start", cliStartMessage(streamer, screenID, confirmed.Title))
	duration, filePath, fileSize, stoppedByUser, recErr := recorder.RecordLiveStreamWithOptions(
		screenID,
		confirmed.Title,
		confirmed.StreamURL,
		cfg.OutputDirectory,
		recAuth,
		rc,
		notifier,
	)
	status := classifyStatus(duration, fileSize, stoppedByUser, recErr, cfg)
	if filePath != "" && (fileSize > 0 || stoppedByUser || recErr != nil) {
		notifier.AddRecordingHistoryWithStatus(screenID, filePath, duration, fileSize, status)
	}
	if recErr != nil {
		if !stoppedByUser {
			sendCLITelegram(cfg, streamer, notifier, "error", cliErrorMessage(streamer, screenID, confirmed.Title, recErr.Error()))
		}
		return recErr
	}
	if !stoppedByUser {
		sendCLITelegram(cfg, streamer, notifier, "finish", cliFinishMessage(streamer, screenID, confirmed.Title, duration, filePath, fileSize))
	}
	fmt.Printf("Recording finished: %s (%s, %d bytes)\n", filePath, duration, fileSize)
	return nil
}

func runDoctor(args []string) error {
	flags, rest, err := parseGlobalFlags("doctor", args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("doctor does not accept positional arguments")
	}
	cfg, err := config.LoadConfig(flags.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	normalizeCLIConfig(cfg)

	fmt.Printf("twitrec %s\n", version)
	fmt.Printf("config: %s\n", flags.configPath)
	fmt.Printf("history: %s\n", paths.HistoryPath(flags.configPath))
	fmt.Printf("logs: %s\n", paths.LogsDir(cfg.OutputDirectory))
	logFile := strings.TrimSpace(flags.logFile)
	if logFile == "" {
		logFile = paths.CLILogPath(cfg.OutputDirectory)
	}
	fmt.Printf("log file: %s\n", logFile)
	fmt.Printf("output: %s\n", cfg.OutputDirectory)
	if err := os.MkdirAll(cfg.OutputDirectory, 0755); err != nil {
		fmt.Printf("[error] output directory: %v\n", err)
	} else {
		fmt.Println("[ok] output directory writable")
	}

	tools := recorder.CheckToolchain(recorder.RecordOptions{
		FFmpegPath:  cfg.Recording.FFmpegPath,
		FFprobePath: cfg.Recording.FFprobePath,
	})
	printTool("ffmpeg", tools.FFmpegOK, tools.FFmpegPath, tools.FFmpegVersion)
	printTool("ffprobe", tools.FFprobeOK, tools.FFprobePath, tools.FFprobeVersion)

	enabled := 0
	for _, streamer := range cfg.Streamers {
		if !streamer.Disabled && strings.TrimSpace(streamer.ScreenID) != "" {
			enabled++
		}
	}
	fmt.Printf("[info] streamers: %d enabled / %d total\n", enabled, len(cfg.Streamers))
	fmt.Printf("[info] check interval: %ds\n", cfg.CheckIntervalSeconds)

	if strings.TrimSpace(cfg.OAuth.AccessToken) == "" {
		fmt.Println("[warn] OAuth access token is empty; official API and comment capture may be limited")
	} else if err := auth.VerifyAccessToken(cfg.OAuth.AccessToken); err != nil {
		fmt.Printf("[warn] OAuth token verify failed: %v\n", err)
	} else {
		fmt.Println("[ok] OAuth token verified")
	}
	if cfg.Recording.SaveCommentsText && strings.TrimSpace(cfg.OAuth.AccessToken) == "" {
		fmt.Println("[warn] comment capture requires OAuth access token")
	}
	return nil
}

func runAuth(args []string) error {
	if len(args) == 0 || args[0] != "verify" {
		return fmt.Errorf("usage: twitcasting-recorder auth verify [--config config.yaml]")
	}
	flags, rest, err := parseGlobalFlags("auth verify", args[1:])
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("auth verify does not accept positional arguments")
	}
	cfg, err := config.LoadConfig(flags.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	token := strings.TrimSpace(cfg.OAuth.AccessToken)
	if token == "" {
		return errors.New("oauth.access_token is empty")
	}
	if err := auth.VerifyAccessToken(token); err != nil {
		return err
	}
	fmt.Println("OAuth token verified.")
	return nil
}

func newCLINotifier(cfg *config.Config, flags *globalFlags) *cliNotifier {
	logFile := strings.TrimSpace(flags.logFile)
	if logFile == "" {
		logFile = paths.CLILogPath(cfg.OutputDirectory)
	}
	return &cliNotifier{
		history:  history.NewHistoryManager(paths.HistoryPath(flags.configPath)),
		logFile:  logFile,
		verbose:  flags.verbose,
		statuses: make(map[string]cliStatus),
	}
}

func (n *cliNotifier) NotifyStatus(screenID, status, message string) {
	n.mu.Lock()
	if n.statuses == nil {
		n.statuses = make(map[string]cliStatus)
	}
	n.statuses[screenID] = cliStatus{
		ScreenID: screenID,
		Status:   status,
		Message:  message,
		Updated:  time.Now(),
	}
	n.mu.Unlock()

	if n.verbose || status == "recording" || status == "error" {
		n.write(fmt.Sprintf("[%s] %s: %s", screenID, status, message))
	}
}

func (n *cliNotifier) NotifyAppLog(message string) {
	n.write(message)
}

func (n *cliNotifier) AddRecordingHistory(streamerID, filePath, duration string, fileSize int64) {
	n.AddRecordingHistoryWithStatus(streamerID, filePath, duration, fileSize, "completed")
}

func (n *cliNotifier) AddRecordingHistoryWithStatus(streamerID, filePath, duration string, fileSize int64, status string) {
	if n.history == nil || strings.TrimSpace(filePath) == "" {
		return
	}
	if status == "" {
		status = "completed"
	}
	now := time.Now()
	record := history.RecordingRecord{
		ID:         fmt.Sprintf("%d-%s", now.UnixNano(), sanitizeID(streamerID)),
		StreamerID: streamerID,
		FilePath:   filePath,
		FileSize:   fileSize,
		Duration:   duration,
		StartTime:  now,
		EndTime:    now,
		Status:     status,
	}
	if err := n.history.AddRecord(record); err != nil {
		n.write(fmt.Sprintf("history write failed: %v", err))
	}
}

func (n *cliNotifier) write(message string) {
	line := fmt.Sprintf("%s %s", time.Now().Format("2006-01-02 15:04:05"), message)
	fmt.Println(line)
	n.mu.Lock()
	n.logs = append([]cliLog{{Time: time.Now(), Message: message}}, n.logs...)
	if len(n.logs) > 300 {
		n.logs = n.logs[:300]
	}
	n.mu.Unlock()

	if strings.TrimSpace(n.logFile) == "" {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(n.logFile), 0755); err != nil {
		return
	}
	rotateFile(n.logFile, 2*1024*1024)
	f, err := os.OpenFile(n.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintln(f, line)
}

func (n *cliNotifier) SnapshotStatuses() map[string]cliStatus {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make(map[string]cliStatus, len(n.statuses))
	for k, v := range n.statuses {
		out[k] = v
	}
	return out
}

func (n *cliNotifier) SnapshotLogs(limit int) []cliLog {
	n.mu.Lock()
	defer n.mu.Unlock()
	if limit <= 0 || limit > len(n.logs) {
		limit = len(n.logs)
	}
	out := make([]cliLog, limit)
	copy(out, n.logs[:limit])
	return out
}

func sendCLITelegram(cfg *config.Config, streamer config.StreamerConfig, notifier *cliNotifier, kind, message string) {
	if cfg == nil {
		return
	}
	if !streamer.TelegramEnabled {
		return
	}
	tg := cfg.Notifications.Telegram
	if !tg.Enabled {
		return
	}
	switch kind {
	case "start":
		if !tg.NotifyOnStart {
			return
		}
	case "finish":
		if !tg.NotifyOnFinish {
			return
		}
	case "error":
		if !tg.NotifyOnError {
			return
		}
	default:
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := notify.SendTelegram(ctx, tg, proxyURL(cfg), message); err != nil && notifier != nil {
		notifier.NotifyAppLog(fmt.Sprintf("telegram push failed: %v", err))
	}
}

func cliStreamerDisplayName(streamer config.StreamerConfig, fallbackScreenID string) string {
	nickname := strings.TrimSpace(streamer.Nickname)
	screenID := strings.TrimSpace(streamer.ScreenID)
	if screenID == "" {
		screenID = strings.TrimSpace(fallbackScreenID)
	}
	if nickname == "" {
		return screenID
	}
	return fmt.Sprintf("%s / %s", nickname, screenID)
}

func cliStartMessage(streamer config.StreamerConfig, screenID, title string) string {
	displayName := cliStreamerDisplayName(streamer, screenID)
	lines := []string{
		fmt.Sprintf("%s 开始直播，并已开始录制", displayName),
		fmt.Sprintf("主播: %s", displayName),
	}
	if strings.TrimSpace(title) != "" {
		lines = append(lines, "标题: "+strings.TrimSpace(title))
	}
	lines = append(lines, "时间: "+time.Now().Format("2006-01-02 15:04:05"))
	return strings.Join(lines, "\n")
}

func cliFinishMessage(streamer config.StreamerConfig, screenID, title, duration, filePath string, fileSize int64) string {
	lines := []string{
		"TwitCasting 录制完成",
		fmt.Sprintf("主播: %s", cliStreamerDisplayName(streamer, screenID)),
	}
	if strings.TrimSpace(title) != "" {
		lines = append(lines, "标题: "+strings.TrimSpace(title))
	}
	if strings.TrimSpace(duration) != "" {
		lines = append(lines, "时长: "+strings.TrimSpace(duration))
	}
	if fileSize > 0 {
		lines = append(lines, "大小: "+formatBytes(fileSize))
	}
	if strings.TrimSpace(filePath) != "" {
		lines = append(lines, "文件: "+filepath.Base(filePath))
	}
	return strings.Join(lines, "\n")
}

func cliErrorMessage(streamer config.StreamerConfig, screenID, title, errMessage string) string {
	lines := []string{
		"TwitCasting 录制失败",
		fmt.Sprintf("主播: %s", cliStreamerDisplayName(streamer, screenID)),
	}
	if strings.TrimSpace(title) != "" {
		lines = append(lines, "标题: "+strings.TrimSpace(title))
	}
	lines = append(lines,
		"错误: "+strings.TrimSpace(errMessage),
		"时间: "+time.Now().Format("2006-01-02 15:04:05"),
	)
	return strings.Join(lines, "\n")
}

func normalizeCLIConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.OutputDirectory) == "" {
		cfg.OutputDirectory = "."
	}
	if cfg.CheckIntervalSeconds <= 0 {
		cfg.CheckIntervalSeconds = 30
	}
	if cfg.CheckIntervalSeconds < 5 {
		cfg.CheckIntervalSeconds = 5
	}
	if cfg.CheckIntervalSeconds > 300 {
		cfg.CheckIntervalSeconds = 300
	}
	if cfg.Recording.WorkerCheckIntervalSeconds <= 0 {
		cfg.Recording.WorkerCheckIntervalSeconds = cfg.CheckIntervalSeconds
	}
}

func defaultSchedule(cfg *config.Config) string {
	return fmt.Sprintf("@every %ds", cfg.CheckIntervalSeconds)
}

func proxyURL(cfg *config.Config) string {
	if cfg.Proxy.Enabled {
		return strings.TrimSpace(cfg.Proxy.URL)
	}
	return ""
}

func authSettings(cfg *config.Config, streamer config.StreamerConfig) recorder.AuthSettings {
	settings := recorder.AuthSettings{
		Mode:          cfg.AuthMode,
		AccessToken:   cfg.OAuth.AccessToken,
		CookieEnabled: cfg.Cookies.Enabled,
		CookieFile:    cfg.Cookies.FilePath,
	}
	switch strings.ToLower(strings.TrimSpace(streamer.AuthMode)) {
	case "cookie":
		settings.Mode = "cookie"
		settings.CookieEnabled = true
	case "no_cookie":
		settings.Mode = "oauth"
		settings.CookieEnabled = false
	}
	return settings
}

func recordOptions(cfg *config.Config, streamer config.StreamerConfig, movieID string) recorder.RecordOptions {
	quality := cfg.Recording.QualityMode
	if strings.TrimSpace(streamer.QualityMode) != "" {
		quality = streamer.QualityMode
	}
	container := cfg.Recording.ContainerMode
	if strings.TrimSpace(streamer.ContainerMode) != "" {
		container = streamer.ContainerMode
	}
	return recorder.RecordOptions{
		QualityMode:          quality,
		ContainerMode:        container,
		SaveInfoText:         cfg.Recording.SaveInfoText,
		SaveCommentsText:     cfg.Recording.SaveCommentsText,
		SaveCommentsTextFile: cfg.Recording.SaveCommentsTextFile,
		CommentTextTemplate:  cfg.Recording.CommentTextTemplate,
		MovieID:              movieID,
		FFmpegPath:           cfg.Recording.FFmpegPath,
		FFprobePath:          cfg.Recording.FFprobePath,
		ProxyURL:             proxyURL(cfg),
	}
}

func findStreamer(cfg *config.Config, screenID string) config.StreamerConfig {
	for _, streamer := range cfg.Streamers {
		if strings.EqualFold(strings.TrimSpace(streamer.ScreenID), screenID) {
			return streamer
		}
	}
	return config.StreamerConfig{}
}

func classifyStatus(duration string, fileSize int64, stoppedByUser bool, recErr error, cfg *config.Config) string {
	if stoppedByUser {
		return "manual_stopped"
	}
	if recErr != nil {
		return "failed"
	}
	if cfg != nil {
		if cfg.Recording.MinFileSizeMB > 0 && fileSize < int64(cfg.Recording.MinFileSizeMB)*1024*1024 {
			return "failed"
		}
		if cfg.Recording.MinDurationSeconds > 0 && parseDurationSeconds(duration) < cfg.Recording.MinDurationSeconds {
			return "failed"
		}
	}
	return "completed"
}

func parseDurationSeconds(value string) int {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	s, _ := strconv.Atoi(parts[2])
	return h*3600 + m*60 + s
}

func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unit])
	}
	return fmt.Sprintf("%.2f %s", value, units[unit])
}

func printTool(name string, ok bool, path string, version string) {
	if ok {
		fmt.Printf("[ok] %s: %s\n", name, path)
		if version != "" {
			fmt.Printf("     %s\n", version)
		}
		return
	}
	fmt.Printf("[error] %s not found: %s\n", name, path)
}

func rotateFile(path string, maxBytes int64) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxBytes {
		return
	}
	backup := strings.TrimSuffix(path, filepath.Ext(path)) + ".1" + filepath.Ext(path)
	_ = os.Remove(backup)
	_ = os.Rename(path, backup)
}

func sanitizeID(s string) string {
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	out := strings.TrimSpace(replacer.Replace(s))
	if out == "" {
		return "unknown"
	}
	return out
}

func printUsage() {
	fmt.Println(`TwitCasting Recorder CLI

Usage:
  twitcasting-recorder monitor --config config.yaml
  twitcasting-recorder web --config config.yaml --addr 127.0.0.1:8787
  twitcasting-recorder record --config config.yaml <screen_id>
  twitcasting-recorder doctor --config config.yaml
  twitcasting-recorder auth verify --config config.yaml
  twitcasting-recorder version

No command defaults to monitor for backwards compatibility.`)
}
