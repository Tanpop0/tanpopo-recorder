package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/config"
	"github.com/user/twitcasting-recorder/apps/gui/pkg/scheduler"
)

type webApp struct {
	cfgPath  string
	cfg      *config.Config
	manager  *scheduler.ValidationManager
	notifier *cliNotifier
	mu       sync.Mutex
}

type webStreamer struct {
	ScreenID   string
	Nickname   string
	Schedule   string
	Disabled   bool
	Monitoring bool
	Status     string
	Message    string
	Updated    string
}

type webPageData struct {
	Version    string
	ConfigPath string
	Addr       string
	OutputDir  string
	Streamers  []webStreamer
	Logs       []cliLog
}

func runWeb(args []string) error {
	fs := flagSet("web")
	flags := &globalFlags{}
	addr := fs.String("addr", "127.0.0.1:8787", "web panel listen address")
	fs.StringVar(&flags.configPath, "config", "config.yaml", "path to config yaml")
	fs.StringVar(&flags.logFile, "log-file", "", "path to CLI log file; default is <output>/logs/cli.log")
	fs.BoolVar(&flags.verbose, "verbose", false, "print verbose status logs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("web does not accept positional arguments")
	}

	cfg, err := config.LoadConfig(flags.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	normalizeCLIConfig(cfg)
	notifier := newCLINotifier(cfg, flags)
	manager := scheduler.NewManager(notifier, cfg)
	app := &webApp{cfgPath: flags.configPath, cfg: cfg, manager: manager, notifier: notifier}

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
		manager.SetMonitoring(streamer.ScreenID, true)
		if cfg.Recording.StartupStaggerSeconds > 0 {
			time.Sleep(time.Duration(cfg.Recording.StartupStaggerSeconds) * time.Second)
		}
	}
	manager.Start()

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleIndex(*addr))
	mux.HandleFunc("/api/status", app.handleStatus)
	mux.HandleFunc("/streamers/add", app.handleAddStreamer)
	mux.HandleFunc("/streamers/remove", app.handleRemoveStreamer)
	mux.HandleFunc("/streamers/start", app.handleStartStreamer)
	mux.HandleFunc("/streamers/pause", app.handlePauseStreamer)

	server := &http.Server{Addr: *addr, Handler: mux}
	go func() {
		notifier.NotifyAppLog(fmt.Sprintf("web panel listening on http://%s (%d streamers)", *addr, added))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			notifier.NotifyAppLog(fmt.Sprintf("web panel failed: %v", err))
		}
	}()

	fmt.Printf("Web panel started: http://%s\nPress Ctrl+C to stop.\n", *addr)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nStopping web panel...")
	manager.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func (a *webApp) handleIndex(addr string) http.HandlerFunc {
	tmpl := template.Must(template.New("index").Parse(webIndexHTML))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		a.mu.Lock()
		cfg := a.cfg
		data := webPageData{
			Version:    version,
			ConfigPath: a.cfgPath,
			Addr:       addr,
			OutputDir:  cfg.OutputDirectory,
			Streamers:  a.streamerRowsLocked(),
			Logs:       a.notifier.SnapshotLogs(80),
		}
		a.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.Execute(w, data)
	}
}

func (a *webApp) handleStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	data := map[string]any{
		"version":   version,
		"streamers": a.streamerRowsLocked(),
		"logs":      a.notifier.SnapshotLogs(80),
	}
	a.mu.Unlock()
	writeJSON(w, data)
}

func (a *webApp) handleAddStreamer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	screenID := strings.TrimSpace(r.FormValue("screen_id"))
	if screenID == "" {
		http.Redirect(w, r, "/?error=screen_id_required", http.StatusSeeOther)
		return
	}
	schedule := strings.TrimSpace(r.FormValue("schedule"))

	a.mu.Lock()
	defer a.mu.Unlock()
	streamer := config.StreamerConfig{
		ScreenID:      screenID,
		Schedule:      schedule,
		QualityMode:   strings.TrimSpace(r.FormValue("quality_mode")),
		ContainerMode: strings.TrimSpace(r.FormValue("container_mode")),
	}
	if streamer.Schedule == "" {
		streamer.Schedule = defaultSchedule(a.cfg)
	}
	if streamer.QualityMode == "" {
		streamer.QualityMode = a.cfg.Recording.QualityMode
	}
	if streamer.ContainerMode == "" {
		streamer.ContainerMode = a.cfg.Recording.ContainerMode
	}
	a.upsertStreamerLocked(streamer)
	if err := a.manager.AddStreamer(streamer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.manager.SetMonitoring(screenID, true)
	_ = config.SaveConfig(a.cfgPath, a.cfg)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *webApp) handleRemoveStreamer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	screenID := strings.TrimSpace(r.FormValue("screen_id"))
	a.mu.Lock()
	defer a.mu.Unlock()
	_ = a.manager.RemoveStreamer(screenID)
	a.removeStreamerLocked(screenID)
	_ = config.SaveConfig(a.cfgPath, a.cfg)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *webApp) handleStartStreamer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	screenID := strings.TrimSpace(r.FormValue("screen_id"))
	a.mu.Lock()
	for i := range a.cfg.Streamers {
		if strings.EqualFold(strings.TrimSpace(a.cfg.Streamers[i].ScreenID), screenID) {
			a.cfg.Streamers[i].Disabled = false
			if strings.TrimSpace(a.cfg.Streamers[i].Schedule) == "" {
				a.cfg.Streamers[i].Schedule = defaultSchedule(a.cfg)
			}
			if err := a.manager.AddStreamer(a.cfg.Streamers[i]); err != nil {
				a.mu.Unlock()
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = config.SaveConfig(a.cfgPath, a.cfg)
			break
		}
	}
	a.mu.Unlock()
	a.manager.SetMonitoring(screenID, true)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *webApp) handlePauseStreamer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	screenID := strings.TrimSpace(r.FormValue("screen_id"))
	a.manager.SetMonitoring(screenID, false)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *webApp) streamerRowsLocked() []webStreamer {
	statuses := a.notifier.SnapshotStatuses()
	rows := make([]webStreamer, 0, len(a.cfg.Streamers))
	for _, streamer := range a.cfg.Streamers {
		screenID := strings.TrimSpace(streamer.ScreenID)
		if screenID == "" {
			continue
		}
		st := statuses[screenID]
		updated := ""
		if !st.Updated.IsZero() {
			updated = st.Updated.Format("15:04:05")
		}
		rows = append(rows, webStreamer{
			ScreenID:   screenID,
			Nickname:   streamer.Nickname,
			Schedule:   streamer.Schedule,
			Disabled:   streamer.Disabled,
			Monitoring: a.manager.IsMonitoring(screenID),
			Status:     fallback(st.Status, "idle"),
			Message:    st.Message,
			Updated:    updated,
		})
	}
	return rows
}

func (a *webApp) upsertStreamerLocked(next config.StreamerConfig) {
	for i := range a.cfg.Streamers {
		if strings.EqualFold(strings.TrimSpace(a.cfg.Streamers[i].ScreenID), next.ScreenID) {
			a.cfg.Streamers[i] = next
			return
		}
	}
	a.cfg.Streamers = append(a.cfg.Streamers, next)
}

func (a *webApp) removeStreamerLocked(screenID string) {
	filtered := a.cfg.Streamers[:0]
	for _, streamer := range a.cfg.Streamers {
		if !strings.EqualFold(strings.TrimSpace(streamer.ScreenID), screenID) {
			filtered = append(filtered, streamer)
		}
	}
	a.cfg.Streamers = filtered
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

const webIndexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="refresh" content="8">
  <title>TwitCasting Recorder Web</title>
  <style>
    :root { color-scheme: light; --bg:#f6f7f9; --panel:#fff; --line:#d9e0ea; --text:#142033; --muted:#65758a; --blue:#2f80ed; --green:#36b37e; --orange:#f5a623; --red:#de350b; }
    * { box-sizing: border-box; }
    body { margin:0; font-family: Arial, sans-serif; background:var(--bg); color:var(--text); }
    header { padding:24px 28px 18px; border-bottom:1px solid var(--line); background:var(--panel); }
    h1 { margin:0; font-size:24px; }
    .sub { margin-top:6px; color:var(--muted); font-size:13px; }
    main { padding:22px 28px 36px; max-width:1180px; }
    .grid { display:grid; grid-template-columns: 1fr 360px; gap:18px; align-items:start; }
    section { background:var(--panel); border:1px solid var(--line); border-radius:8px; overflow:hidden; }
    h2 { margin:0; padding:15px 16px; border-bottom:1px solid var(--line); font-size:16px; }
    table { width:100%; border-collapse:collapse; }
    th, td { padding:12px 14px; border-bottom:1px solid var(--line); text-align:left; font-size:14px; vertical-align:middle; }
    th { color:var(--muted); font-size:12px; font-weight:600; background:#fafbfc; }
    tr:last-child td { border-bottom:0; }
    .badge { display:inline-block; padding:3px 8px; border-radius:999px; background:#edf2f7; font-size:12px; color:#42526e; }
    .recording { background:#ffe7e6; color:var(--red); }
    .monitoring { background:#e3fcef; color:#00875a; }
    .error { background:#fff0b3; color:#974f0c; }
    .actions { display:flex; gap:8px; flex-wrap:wrap; }
    button { border:1px solid var(--line); background:#fff; border-radius:6px; padding:7px 10px; cursor:pointer; font-weight:600; }
    button.primary { border-color:var(--blue); background:var(--blue); color:#fff; }
    button.green { border-color:var(--green); background:var(--green); color:#fff; }
    button.orange { border-color:var(--orange); background:var(--orange); color:#fff; }
    button.danger { border-color:#ffbdad; color:var(--red); }
    form.add { display:grid; grid-template-columns: 1fr 140px 120px 120px auto; gap:10px; padding:16px; border-bottom:1px solid var(--line); }
    input, select { width:100%; border:1px solid var(--line); border-radius:6px; padding:8px 10px; font-size:14px; }
    .logs { padding:12px 14px; height:520px; overflow:auto; background:#101418; color:#d6e4ff; font-family: Consolas, monospace; font-size:12px; }
    .logline { margin-bottom:7px; white-space:pre-wrap; }
    .empty { padding:26px 16px; color:var(--muted); text-align:center; }
    @media (max-width: 920px) { .grid { grid-template-columns:1fr; } form.add { grid-template-columns:1fr; } }
  </style>
</head>
<body>
  <header>
    <h1>TwitCasting Recorder Web</h1>
    <div class="sub">Version {{.Version}} | {{.Addr}} | Config {{.ConfigPath}} | Output {{.OutputDir}}</div>
  </header>
  <main>
    <div class="grid">
      <section>
        <h2>Streamers</h2>
        <form class="add" method="post" action="/streamers/add">
          <input name="screen_id" placeholder="screen id, e.g. c:example" required>
          <input name="schedule" placeholder="@every 30s">
          <select name="quality_mode">
            <option value="stable">stable</option>
            <option value="auto">auto</option>
            <option value="original">original</option>
          </select>
          <select name="container_mode">
            <option value="mkv">mkv</option>
            <option value="ts">ts</option>
            <option value="mp4">mp4</option>
          </select>
          <button class="primary" type="submit">Add</button>
        </form>
        {{if .Streamers}}
        <table>
          <thead><tr><th>ID</th><th>Schedule</th><th>Status</th><th>Message</th><th>Actions</th></tr></thead>
          <tbody>
          {{range .Streamers}}
            <tr>
              <td><strong>{{.ScreenID}}</strong>{{if .Nickname}}<br><span class="sub">{{.Nickname}}</span>{{end}}</td>
              <td>{{.Schedule}}</td>
              <td><span class="badge {{.Status}}">{{.Status}}</span>{{if .Updated}}<br><span class="sub">{{.Updated}}</span>{{end}}</td>
              <td>{{.Message}}</td>
              <td>
                <div class="actions">
                  <form method="post" action="/streamers/start"><input type="hidden" name="screen_id" value="{{.ScreenID}}"><button class="green" type="submit">Start</button></form>
                  <form method="post" action="/streamers/pause"><input type="hidden" name="screen_id" value="{{.ScreenID}}"><button class="orange" type="submit">Pause</button></form>
                  <form method="post" action="/streamers/remove" onsubmit="return confirm('Remove {{.ScreenID}} from config?')"><input type="hidden" name="screen_id" value="{{.ScreenID}}"><button class="danger" type="submit">Remove</button></form>
                </div>
              </td>
            </tr>
          {{end}}
          </tbody>
        </table>
        {{else}}
        <div class="empty">No streamers configured.</div>
        {{end}}
      </section>
      <section>
        <h2>Logs</h2>
        <div class="logs">
          {{range .Logs}}<div class="logline">[{{.Time.Format "15:04:05"}}] {{.Message}}</div>{{else}}No logs yet.{{end}}
        </div>
      </section>
    </div>
  </main>
</body>
</html>`
