package recorder

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	stdruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/comments"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/cookies"
)

// StatusNotifier avoids circular dependency.
type StatusNotifier interface {
	NotifyStatus(screenID, status, message string)
	NotifyAppLog(message string)
}

// AuthSettings controls auth strategy for recorder.
type AuthSettings struct {
	Mode          string
	AccessToken   string
	CookieFile    string
	CookieEnabled bool
}

type RecordOptions struct {
	QualityMode          string
	ContainerMode        string
	SaveInfoText         bool
	SaveCommentsText     bool
	SaveCommentsTextFile bool
	CommentTextTemplate  string
	MovieID              string
	FFmpegPath           string
	FFprobePath          string
	ProxyURL             string
}

type ToolStatus struct {
	FFmpegPath     string `json:"ffmpeg_path"`
	FFmpegOK       bool   `json:"ffmpeg_ok"`
	FFmpegVersion  string `json:"ffmpeg_version"`
	FFprobePath    string `json:"ffprobe_path"`
	FFprobeOK      bool   `json:"ffprobe_ok"`
	FFprobeVersion string `json:"ffprobe_version"`
	Message        string `json:"message"`
}

type recordingHandle struct {
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stopRequested atomic.Bool
}

func sanitizeFilename(s string) string {
	invalid := []string{":", "/", "\\", "*", "?", "\"", "<", ">", "|"}
	result := s
	for _, c := range invalid {
		result = strings.ReplaceAll(result, c, "_")
	}
	if len(result) > 50 {
		result = result[:50]
	}
	return strings.TrimSpace(result)
}

func normalizeID(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

var (
	activeCmds sync.Map // map[string]*recordingHandle
)

var hlsVariantPattern = regexp.MustCompile(`(/hls/)(\d+\.\d+)(/media(?:\.\d+)?\.m3u8)`)

const (
	startupNoMediaProgressTimeout = 2 * time.Minute
	mediaProgressStallTimeout     = 3 * time.Minute
)

func toolExecutableNames(toolName string) []string {
	if stdruntime.GOOS == "windows" {
		return []string{toolName + ".exe", toolName}
	}
	return []string{toolName}
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func absIfPossible(path string) string {
	if absPath, err := filepath.Abs(path); err == nil {
		return absPath
	}
	return path
}

func resolveToolInDir(dir string, toolName string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	for _, exeName := range toolExecutableNames(toolName) {
		for _, candidate := range []string{
			filepath.Join(dir, exeName),
			filepath.Join(dir, "bin", exeName),
		} {
			if regularFile(candidate) {
				return absIfPossible(candidate)
			}
		}
	}
	return ""
}

func resolvePreferredToolPath(preferredPath string, toolName string) string {
	preferredPath = strings.TrimSpace(preferredPath)
	if preferredPath == "" {
		return ""
	}

	if info, err := os.Stat(preferredPath); err == nil {
		if info.IsDir() {
			return resolveToolInDir(preferredPath, toolName)
		}
		return preferredPath
	}

	for _, exeName := range toolExecutableNames(toolName) {
		for _, candidate := range []string{
			preferredPath + filepath.Ext(exeName),
			filepath.Join(preferredPath, exeName),
			filepath.Join(preferredPath, "bin", exeName),
		} {
			if regularFile(candidate) {
				return absIfPossible(candidate)
			}
		}
	}

	return ""
}

func findToolPath(preferredPath string, toolName string) string {
	if resolved := resolvePreferredToolPath(preferredPath, toolName); resolved != "" {
		return resolved
	}
	if path, err := exec.LookPath(toolName); err == nil {
		return path
	}
	for _, exeName := range toolExecutableNames(toolName) {
		if regularFile(exeName) {
			return absIfPossible(exeName)
		}
	}
	return toolName
}

func findFFmpegPath(preferredPath string) string {
	return findToolPath(preferredPath, "ffmpeg")
}

func findFFprobePath(preferredPath string) string {
	return findToolPath(preferredPath, "ffprobe")
}

func CheckToolchain(options RecordOptions) ToolStatus {
	status := ToolStatus{
		FFmpegPath:  findFFmpegPath(options.FFmpegPath),
		FFprobePath: findFFprobePath(options.FFprobePath),
	}

	status.FFmpegOK, status.FFmpegVersion = probeToolVersion(status.FFmpegPath)
	status.FFprobeOK, status.FFprobeVersion = probeToolVersion(status.FFprobePath)
	switch {
	case status.FFmpegOK && status.FFprobeOK:
		status.Message = "FFmpeg and FFprobe are ready"
	case status.FFmpegOK:
		status.Message = "FFmpeg is ready, but FFprobe was not found"
	default:
		status.Message = "FFmpeg was not found. Install FFmpeg or set its path in settings"
	}
	return status
}

func probeToolVersion(path string) (bool, string) {
	cmd := exec.Command(path, "-version")
	prepareCommand(cmd)
	out, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return true, ""
	}
	return true, strings.TrimSpace(lines[0])
}

type containerPlan struct {
	mode         string
	tempExt      string
	finalExt     string
	ffmpegFormat string
	remuxToMP4   bool
}

func resolveContainerPlan(containerMode string) containerPlan {
	switch strings.ToLower(strings.TrimSpace(containerMode)) {
	case "ts":
		return containerPlan{mode: "ts", tempExt: ".ts", finalExt: ".ts", ffmpegFormat: "mpegts"}
	case "mp4":
		return containerPlan{mode: "mp4", tempExt: ".ts", finalExt: ".mp4", ffmpegFormat: "mpegts", remuxToMP4: true}
	default:
		return containerPlan{mode: "mkv", tempExt: ".mkv", finalExt: ".mkv", ffmpegFormat: "matroska"}
	}
}

func formatDurationHMS(totalSeconds int64) string {
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	h := totalSeconds / 3600
	m := (totalSeconds % 3600) / 60
	s := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func normalizeMediaTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.Split(raw, ".")[0]
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return ""
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	s, errS := strconv.Atoi(parts[2])
	if errH != nil || errM != nil || errS != nil {
		return ""
	}
	if h < 0 || m < 0 || s < 0 {
		return ""
	}
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func probeFileDuration(filePath string, ffprobePath string) string {
	if strings.TrimSpace(filePath) == "" {
		return ""
	}
	if _, err := os.Stat(filePath); err != nil {
		return ""
	}

	cmd := exec.Command(findFFprobePath(ffprobePath),
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1",
		filePath,
	)
	prepareCommand(cmd)

	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(out))
	if text == "" || text == "N/A" {
		return ""
	}

	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return ""
	}
	if f < 0 {
		f = 0
	}
	return formatDurationHMS(int64(f + 0.5))
}

type mediaProbeInfo struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

func detectLoginPlaceholderRecording(filePath string, fileSize int64, ffprobePath string) error {
	if strings.TrimSpace(filePath) == "" || fileSize <= 0 {
		return nil
	}
	cmd := exec.Command(findFFprobePath(ffprobePath),
		"-v", "error",
		"-show_entries", "stream=codec_type,width,height:format=duration,bit_rate",
		"-of", "json",
		filePath,
	)
	prepareCommand(cmd)

	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var info mediaProbeInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(info.Format.Duration), 64)
	if err != nil || duration < 600 {
		return nil
	}

	bitRate, _ := strconv.ParseInt(strings.TrimSpace(info.Format.BitRate), 10, 64)
	if bitRate <= 0 {
		bitRate = int64(float64(fileSize*8) / duration)
	}
	if bitRate <= 0 || bitRate > 60000 {
		return nil
	}

	hasLargeVideo := false
	for _, stream := range info.Streams {
		if stream.CodecType == "video" && stream.Width >= 640 && stream.Height >= 360 {
			hasLargeVideo = true
			break
		}
	}
	if !hasLargeVideo {
		return nil
	}

	return fmt.Errorf("possible login-required placeholder recording detected (bitrate %.1f kbit/s); refresh cookies.txt or verify this account can watch the streamer", float64(bitRate)/1000)
}

func remuxTSFileToMP4(inputPath, outputPath, ffmpegPath string) error {
	if strings.TrimSpace(inputPath) == "" || strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("input or output path is empty")
	}
	cmd := exec.Command(findFFmpegPath(ffmpegPath),
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", inputPath,
		"-c", "copy",
		"-movflags", "+faststart",
		outputPath,
	)
	prepareCommand(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func writeInfoTextFile(filePath, streamerID, title, streamURL, qualityMode, containerMode, duration string, fileSize int64, startTime, endTime time.Time, stoppedByUser bool, recErr error) {
	if strings.TrimSpace(filePath) == "" {
		return
	}
	infoPath := strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".txt"
	status := "completed"
	if stoppedByUser {
		status = "interrupted"
	} else if recErr != nil {
		status = "failed"
	}
	lines := []string{
		"TwitCasting Recording Info",
		fmt.Sprintf("Streamer ID: %s", streamerID),
		fmt.Sprintf("Title: %s", title),
		fmt.Sprintf("Status: %s", status),
		fmt.Sprintf("Start Time: %s", startTime.Format(time.RFC3339)),
		fmt.Sprintf("End Time: %s", endTime.Format(time.RFC3339)),
		fmt.Sprintf("Duration: %s", duration),
		fmt.Sprintf("File Path: %s", filePath),
		fmt.Sprintf("File Size: %d", fileSize),
		fmt.Sprintf("Quality Mode: %s", qualityMode),
		fmt.Sprintf("Container Mode: %s", containerMode),
		fmt.Sprintf("Stream URL: %s", streamURL),
	}
	if recErr != nil {
		lines = append(lines, fmt.Sprintf("Error: %v", recErr))
	}
	_ = os.WriteFile(infoPath, []byte(strings.Join(lines, "\r\n")+"\r\n"), 0644)
}

func buildFFmpegCookieJar(cookieFile string) string {
	return cookies.BuildFFmpegCookieJar(cookieFile)
}

func cookieAuthEnabled(auth AuthSettings) bool {
	mode := strings.ToLower(strings.TrimSpace(auth.Mode))
	if mode == "" {
		mode = "auto"
	}
	return auth.CookieEnabled && (mode == "cookie" || mode == "auto")
}

// IsRestrictedAccessError reports whether a recording failure indicates that
// the resolved live stream exists but the current account cannot open it.
func IsRestrictedAccessError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "401") ||
		strings.Contains(lower, "403") ||
		strings.Contains(lower, "404") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "forbidden")
}

func buildFFmpegHTTPOptions(streamerID string, auth AuthSettings) (string, string) {
	headers := []string{
		fmt.Sprintf("Referer: https://twitcasting.tv/%s", streamerID),
		"Origin: https://twitcasting.tv",
	}
	cookieJar := ""
	if cookieAuthEnabled(auth) {
		cookieJar = buildFFmpegCookieJar(auth.CookieFile)
	}
	return strings.Join(headers, "\r\n") + "\r\n", cookieJar
}

func redactFFmpegArgs(args []string) string {
	redacted := append([]string(nil), args...)
	for i := 0; i < len(redacted)-1; i++ {
		switch redacted[i] {
		case "-headers", "-cookies":
			redacted[i+1] = "<redacted>"
		}
	}
	return strings.Join(redacted, " ")
}

func newHTTPClient(timeout time.Duration, proxyRawURL string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	proxyRawURL = strings.TrimSpace(proxyRawURL)
	if proxyRawURL != "" {
		if u, err := url.Parse(proxyRawURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

func probePlaylistURL(candidateURL, streamerID, proxyRawURL string) bool {
	client := newHTTPClient(2500*time.Millisecond, proxyRawURL)
	req, err := http.NewRequest("GET", candidateURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	req.Header.Set("Referer", fmt.Sprintf("https://twitcasting.tv/%s", streamerID))

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func shouldForwardProcessLog(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return false
	}

	noisyParts := []string{
		"progress=",
		"frame=",
		"fps=",
		"bitrate=",
		"total_size=",
		"out_time",
		"dup_frames=",
		"drop_frames=",
		"speed=",
		"stream_0_0_q=",
		"opening 'http",
		"skip ('#ext",
		"skipping ",
		"[hls @",
		"[https @",
		"[tls @",
		"stream mapping:",
		"metadata:",
		"duration: n/a",
		"program 0",
		"stream #",
		"major_brand",
		"minor_version",
		"compatible_brands",
		"variant_bitrate",
		"press [q] to stop",
		"last message repeated",
	}
	for _, part := range noisyParts {
		if strings.Contains(lower, part) {
			return false
		}
	}

	importantParts := []string{
		"error",
		"failed",
		"invalid",
		"unauthorized",
		"forbidden",
		"reconnect",
		"exiting",
		"end of file",
		"input/output error",
		"403",
		"404",
		"429",
		"500",
		"503",
	}
	for _, part := range importantParts {
		if strings.Contains(lower, part) {
			return true
		}
	}

	return false
}

func pickStableVariantURL(streamURL, streamerID, qualityMode string, proxyRawURL ...string) string {
	proxyURL := ""
	if len(proxyRawURL) > 0 {
		proxyURL = proxyRawURL[0]
	}
	mode := strings.ToLower(strings.TrimSpace(qualityMode))
	if mode == "" {
		mode = "stable"
	}
	if mode == "original" {
		return streamURL
	}
	m := hlsVariantPattern.FindStringSubmatch(streamURL)
	if len(m) < 4 {
		return streamURL
	}

	currentVariant := m[2]
	if currentVariant == "604.96" {
		return streamURL
	}

	candidates := []string{"604.96"}
	if mode == "auto" {
		candidates = []string{"705.00", "604.96", "432.00"}
	}
	for _, v := range candidates {
		candidateURL := strings.Replace(streamURL, m[0], m[1]+v+m[3], 1)
		if candidateURL == streamURL {
			continue
		}
		if probePlaylistURL(candidateURL, streamerID, proxyURL) {
			return candidateURL
		}
	}

	return streamURL
}

// RecordLiveStream records from a resolved live stream URL using ffmpeg.
// Returns duration, filePath, fileSize, stoppedByUser, error.
func RecordLiveStream(streamerID, title, streamURL, outputDir string, auth AuthSettings, notifier StatusNotifier) (string, string, int64, bool, error) {
	return RecordLiveStreamWithOptions(streamerID, title, streamURL, outputDir, auth, RecordOptions{}, notifier)
}

// RecordLiveStreamWithOptions records from a resolved live stream URL using ffmpeg and explicit stability options.
func RecordLiveStreamWithOptions(streamerID, title, streamURL, outputDir string, auth AuthSettings, options RecordOptions, notifier StatusNotifier) (string, string, int64, bool, error) {
	originalStreamURL := streamURL
	streamURL = pickStableVariantURL(streamURL, streamerID, options.QualityMode, options.ProxyURL)
	if notifier != nil && streamURL != originalStreamURL {
		notifier.NotifyAppLog(fmt.Sprintf("[%s] Switched to stable variant stream URL", streamerID))
	}

	sanitizedID := sanitizeFilename(streamerID)
	if sanitizedID == "" {
		sanitizedID = "unknown"
	}

	sanitizedTitle := sanitizeFilename(title)
	if sanitizedTitle == "" {
		sanitizedTitle = "untitled"
	}
	plan := resolveContainerPlan(options.ContainerMode)

	streamerDir := filepath.Join(outputDir, sanitizedID)
	if err := os.MkdirAll(streamerDir, 0755); err != nil {
		return "", "", 0, false, fmt.Errorf("mkdir failed: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	tempFileName := fmt.Sprintf("%s_temp_%s%s", sanitizedID, timestamp, plan.tempExt)
	tempOutputPath := filepath.Join(streamerDir, tempFileName)
	finalFileName := fmt.Sprintf("%s_%s_%s%s", sanitizedID, sanitizedTitle, timestamp, plan.finalExt)
	finalOutputPath := filepath.Join(streamerDir, finalFileName)

	headerArg, ffmpegCookieJar := buildFFmpegHTTPOptions(streamerID, auth)
	if cookieAuthEnabled(auth) && notifier != nil {
		if ffmpegCookieJar != "" {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Cookie auth enabled through FFmpeg cookie jar for HLS requests", streamerID))
		} else {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Cookie auth requested but no usable cookies were found at %s", streamerID, auth.CookieFile))
		}
	}

	ffmpegArgs := []string{
		"-hide_banner",
		"-loglevel", "info",
		"-y",
		"-user_agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
		"-headers", headerArg,
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "5",
		"-rw_timeout", "15000000",
		"-analyzeduration", "100M",
		"-probesize", "100M",
		"-err_detect", "ignore_err",
	}
	if ffmpegCookieJar != "" {
		ffmpegArgs = append(ffmpegArgs, "-cookies", ffmpegCookieJar)
	}
	if strings.TrimSpace(options.ProxyURL) != "" {
		ffmpegArgs = append(ffmpegArgs, "-http_proxy", strings.TrimSpace(options.ProxyURL))
	}
	ffmpegArgs = append(ffmpegArgs,
		"-i", streamURL,
		"-map", "0:v:0?",
		"-map", "0:a:0?",
		"-c", "copy",
		"-f", plan.ffmpegFormat,
		"-progress", "pipe:1",
		"-nostats",
		tempOutputPath,
	)

	cmd := exec.Command(findFFmpegPath(options.FFmpegPath), ffmpegArgs...)
	prepareCommand(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", "", 0, false, fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", "", 0, false, fmt.Errorf("failed to get stderr pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", 0, false, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	normID := normalizeID(streamerID)
	h := &recordingHandle{cmd: cmd, stdin: stdin}
	activeCmds.Store(normID, h)
	defer activeCmds.Delete(normID)

	fmt.Printf("Starting ffmpeg for %s...\nStream URL: %s\nCommand: %s %s\n", streamerID, streamURL, findFFmpegPath(options.FFmpegPath), redactFFmpegArgs(ffmpegArgs))
	if notifier != nil {
		notifier.NotifyAppLog(fmt.Sprintf("[%s] Recording process starting", streamerID))
	}
	if err := cmd.Start(); err != nil {
		return "", "", 0, false, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	startTime := time.Now()
	commentCancel, commentDone := startCommentCapture(streamerID, finalOutputPath, startTime, auth, options, notifier)
	defer func() {
		if commentCancel != nil {
			commentCancel()
			<-commentDone
		}
	}()
	var wg sync.WaitGroup
	var lastProgressUpdate time.Time
	var lastProcessLogUpdate time.Time
	var progressMu sync.Mutex
	var logMu sync.Mutex
	var restrictedErrMu sync.Mutex
	var stallMu sync.Mutex
	lastRestrictedAccessLine := ""
	lastMediaTime := ""
	lastMediaProgressAt := startTime
	sawMediaProgress := false
	stallReason := ""
	timeRegex := regexp.MustCompile(`time=([0-9:.]+)`)
	outTimeRegex := regexp.MustCompile(`out_time=([0-9:.]+)`)

	notifyProgress := func(mediaTime string) {
		mediaTime = normalizeMediaTime(mediaTime)
		if mediaTime == "" || notifier == nil {
			return
		}

		progressMu.Lock()
		lastMediaTime = mediaTime
		lastMediaProgressAt = time.Now()
		sawMediaProgress = true
		shouldNotify := time.Since(lastProgressUpdate) >= 1*time.Second
		if shouldNotify {
			lastProgressUpdate = time.Now()
		}
		progressMu.Unlock()

		if shouldNotify {
			notifier.NotifyStatus(streamerID, "recording", fmt.Sprintf("Recording %s", mediaTime))
		}
	}

	readPipe := func(scanner *bufio.Scanner, name string) {
		defer wg.Done()
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			fmt.Printf("[FFmpeg-%s] %s\n", name, line)
			if IsRestrictedAccessError(fmt.Errorf("%s", line)) {
				restrictedErrMu.Lock()
				lastRestrictedAccessLine = line
				restrictedErrMu.Unlock()
			}
			if notifier != nil && shouldForwardProcessLog(line) {
				logMu.Lock()
				shouldNotifyLog := time.Since(lastProcessLogUpdate) >= 500*time.Millisecond
				if shouldNotifyLog {
					lastProcessLogUpdate = time.Now()
				}
				logMu.Unlock()
				if shouldNotifyLog {
					notifier.NotifyAppLog(fmt.Sprintf("[%s][%s] %s", streamerID, name, line))
				}
			}

			if h.stopRequested.Load() {
				continue
			}

			if m := outTimeRegex.FindStringSubmatch(line); len(m) > 1 {
				notifyProgress(m[1])
				continue
			}
			if m := timeRegex.FindStringSubmatch(line); len(m) > 1 {
				notifyProgress(m[1])
				continue
			}

			if notifier != nil && strings.Contains(strings.ToLower(line), "error") {
				progressMu.Lock()
				shouldNotify := time.Since(lastProgressUpdate) >= 1*time.Second
				if shouldNotify {
					lastProgressUpdate = time.Now()
				}
				progressMu.Unlock()
				if shouldNotify {
					notifier.NotifyStatus(streamerID, "recording", "Error: "+line)
				}
			}
		}
	}

	stderrScanner := bufio.NewScanner(stderr)
	stdoutScanner := bufio.NewScanner(stdout)
	stderrScanner.Buffer(make([]byte, 1024), 1024*1024)
	stdoutScanner.Buffer(make([]byte, 1024), 1024*1024)

	wg.Add(2)
	go readPipe(stderrScanner, "stderr")
	go readPipe(stdoutScanner, "stdout")

	watchdogDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-watchdogDone:
				return
			case <-ticker.C:
				if h.stopRequested.Load() {
					continue
				}

				progressMu.Lock()
				progressSeen := sawMediaProgress
				lastProgress := lastMediaProgressAt
				progressMu.Unlock()

				reason := ""
				if !progressSeen && time.Since(startTime) >= startupNoMediaProgressTimeout {
					reason = fmt.Sprintf("no media progress for %s after start", startupNoMediaProgressTimeout)
				} else if progressSeen && time.Since(lastProgress) >= mediaProgressStallTimeout {
					reason = fmt.Sprintf("media progress stalled for %s", mediaProgressStallTimeout)
				}
				if reason == "" {
					continue
				}

				stallMu.Lock()
				if stallReason == "" {
					stallReason = reason
				}
				alreadyMarked := stallReason != reason
				stallMu.Unlock()
				if alreadyMarked {
					return
				}

				if notifier != nil {
					notifier.NotifyAppLog(fmt.Sprintf("[%s] FFmpeg media progress stalled: %s; stopping current recorder", streamerID, reason))
				}
				if h.stdin != nil {
					_, _ = io.WriteString(h.stdin, "q\n")
					_ = h.stdin.Close()
				}
				if h.cmd.Process != nil {
					go func(cmd *exec.Cmd) {
						time.Sleep(8 * time.Second)
						_ = cmd.Process.Kill()
					}(h.cmd)
				}
				return
			}
		}
	}()

	err = cmd.Wait()
	close(watchdogDone)
	wg.Wait()

	if _, statErr := os.Stat(tempOutputPath); statErr == nil && plan.remuxToMP4 {
		if remuxErr := remuxTSFileToMP4(tempOutputPath, finalOutputPath, options.FFmpegPath); remuxErr != nil {
			fmt.Printf("Failed to remux mp4, keeping temp file: %v\n", remuxErr)
			if notifier != nil {
				notifier.NotifyAppLog(fmt.Sprintf("[%s] MP4 remux failed, kept TS file: %v", streamerID, remuxErr))
			}
			finalOutputPath = tempOutputPath
		} else {
			_ = os.Remove(tempOutputPath)
		}
	} else if _, statErr := os.Stat(tempOutputPath); statErr == nil {
		maxRetries := 10
		renamed := false
		for i := 0; i < maxRetries; i++ {
			if renameErr := os.Rename(tempOutputPath, finalOutputPath); renameErr != nil {
				low := strings.ToLower(renameErr.Error())
				if strings.Contains(low, "used by another process") || strings.Contains(low, "access is denied") {
					time.Sleep(2 * time.Second)
					continue
				}
				fmt.Printf("Failed to rename file: %v\n", renameErr)
				break
			}
			renamed = true
			break
		}
		if !renamed {
			finalOutputPath = tempOutputPath
		}
	} else {
		finalOutputPath = tempOutputPath
	}

	elapsed := time.Since(startTime)
	elapsedStr := formatDurationHMS(int64(elapsed.Seconds()))

	var fileSize int64
	if info, statErr := os.Stat(finalOutputPath); statErr == nil {
		fileSize = info.Size()
	}

	progressMu.Lock()
	capturedMediaTime := lastMediaTime
	progressMu.Unlock()

	durationStr := elapsedStr
	if media := normalizeMediaTime(capturedMediaTime); media != "" && media != "00:00:00" {
		durationStr = media
	} else if fileSize > 0 {
		if probed := probeFileDuration(finalOutputPath, options.FFprobePath); probed != "" {
			durationStr = probed
		}
	}

	stoppedByUser := h.stopRequested.Load()
	mediaStallError := func() error {
		stallMu.Lock()
		defer stallMu.Unlock()
		if strings.TrimSpace(stallReason) == "" || stoppedByUser {
			return nil
		}
		return fmt.Errorf("ffmpeg media progress stalled: %s", stallReason)
	}
	withRestrictedAccessDetail := func(base error) error {
		restrictedErrMu.Lock()
		detail := lastRestrictedAccessLine
		restrictedErrMu.Unlock()
		if base == nil || strings.TrimSpace(detail) == "" {
			return base
		}
		return fmt.Errorf("%w: %s", base, detail)
	}
	writeInfo := func(stopped bool, recErr error) {
		if options.SaveInfoText {
			writeInfoTextFile(finalOutputPath, streamerID, title, streamURL, options.QualityMode, plan.mode, durationStr, fileSize, startTime, time.Now(), stopped, recErr)
		}
	}
	if err != nil {
		if stoppedByUser || strings.Contains(err.Error(), "signal: killed") {
			fmt.Printf("Recorder stopped by user (recorded %s)\n", durationStr)
			if notifier != nil {
				notifier.NotifyAppLog(fmt.Sprintf("[%s] Stopped by user, duration %s", streamerID, durationStr))
			}
			writeInfo(true, nil)
			return durationStr, finalOutputPath, fileSize, true, nil
		}
		if fileSize > 0 {
			fmt.Printf("ffmpeg exited non-zero but file exists (recorded %s): %v\n", durationStr, err)
			if notifier != nil {
				notifier.NotifyAppLog(fmt.Sprintf("[%s] Exited non-zero, duration %s", streamerID, durationStr))
			}
			recErr := withRestrictedAccessDetail(fmt.Errorf("ffmpeg exited non-zero: %w", err))
			if stallErr := mediaStallError(); stallErr != nil {
				recErr = stallErr
			}
			writeInfo(false, recErr)
			return durationStr, finalOutputPath, fileSize, false, recErr
		}
		recErr := withRestrictedAccessDetail(fmt.Errorf("ffmpeg execution failed: %w", err))
		if stallErr := mediaStallError(); stallErr != nil {
			recErr = stallErr
		}
		writeInfo(false, recErr)
		return durationStr, finalOutputPath, fileSize, false, recErr
	}

	if stallErr := mediaStallError(); stallErr != nil {
		if notifier != nil {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Recording stopped after media progress stall, duration %s", streamerID, durationStr))
		}
		writeInfo(false, stallErr)
		return durationStr, finalOutputPath, fileSize, false, stallErr
	}

	if placeholderErr := detectLoginPlaceholderRecording(finalOutputPath, fileSize, options.FFprobePath); placeholderErr != nil {
		if notifier != nil {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Recording content check failed: %v", streamerID, placeholderErr))
		}
		writeInfo(stoppedByUser, placeholderErr)
		return durationStr, finalOutputPath, fileSize, stoppedByUser, placeholderErr
	}

	fmt.Printf("ffmpeg finished %s (duration: %s)\n", streamerID, durationStr)
	if notifier != nil {
		notifier.NotifyAppLog(fmt.Sprintf("[%s] Recording finished, duration %s", streamerID, durationStr))
	}
	writeInfo(stoppedByUser, nil)
	return durationStr, finalOutputPath, fileSize, stoppedByUser, nil
}

func startCommentCapture(streamerID, finalOutputPath string, startTime time.Time, auth AuthSettings, options RecordOptions, notifier StatusNotifier) (context.CancelFunc, <-chan struct{}) {
	done := make(chan struct{})
	if !options.SaveCommentsText {
		close(done)
		return nil, done
	}

	movieID := strings.TrimSpace(options.MovieID)
	token := strings.TrimSpace(auth.AccessToken)
	if movieID == "" {
		if notifier != nil {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Comment capture skipped: movie_id is empty", streamerID))
		}
		close(done)
		return nil, done
	}
	if token == "" {
		if notifier != nil {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Comment capture skipped: access_token is empty", streamerID))
		}
		close(done)
		return nil, done
	}

	basePath := strings.TrimSuffix(finalOutputPath, filepath.Ext(finalOutputPath))
	textPath := ""
	if options.SaveCommentsTextFile {
		textPath = basePath + ".comments.txt"
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(done)
		if notifier != nil {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Comment capture started", streamerID))
		}
		err := comments.Capture(ctx, comments.Options{
			MovieID:      movieID,
			AccessToken:  token,
			TextPath:     textPath,
			JSONLPath:    basePath + ".comments.jsonl",
			StartTime:    startTime,
			TextTemplate: options.CommentTextTemplate,
			ProxyURL:     options.ProxyURL,
			Logf: func(format string, args ...any) {
				if notifier != nil {
					notifier.NotifyAppLog(fmt.Sprintf("[%s] "+format, append([]any{streamerID}, args...)...))
				}
			},
		})
		if err != nil && notifier != nil {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Comment capture stopped: %v", streamerID, err))
			return
		}
		if notifier != nil {
			notifier.NotifyAppLog(fmt.Sprintf("[%s] Comment capture stopped", streamerID))
		}
	}()
	return cancel, done
}

func findHandle(streamerID string) (*recordingHandle, bool) {
	normID := normalizeID(streamerID)
	if v, ok := activeCmds.Load(normID); ok {
		if h, okCast := v.(*recordingHandle); okCast && h != nil {
			return h, true
		}
	}

	var found *recordingHandle
	activeCmds.Range(func(k, v any) bool {
		key, okKey := k.(string)
		h, okHandle := v.(*recordingHandle)
		if !okKey || !okHandle || h == nil {
			return true
		}
		if normalizeID(key) == normID {
			found = h
			return false
		}
		return true
	})
	if found != nil {
		return found, true
	}
	return nil, false
}

// StopRecording requests graceful stop first, then force kills as fallback.
func StopRecording(streamerID string) error {
	h, ok := findHandle(streamerID)
	if !ok {
		return fmt.Errorf("no active recording found for %s", streamerID)
	}
	if h == nil || h.cmd == nil {
		return fmt.Errorf("invalid recording handle")
	}

	h.stopRequested.Store(true)

	if h.stdin != nil {
		_, _ = io.WriteString(h.stdin, "q\n")
		_ = h.stdin.Close()
	}

	if h.cmd.Process != nil {
		go func(cmd *exec.Cmd) {
			time.Sleep(6 * time.Second)
			_ = cmd.Process.Kill()
		}(h.cmd)
	}

	return nil
}
