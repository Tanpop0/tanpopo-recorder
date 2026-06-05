# TwitCasting Recorder

TwitCasting Recorder is a desktop and server-friendly recorder for TwitCasting
live streams. It provides a Wails/Vue GUI for daily desktop use, plus a CLI,
worker process, and lightweight Web panel for long-running monitoring.

The project is designed around one recording unit per streamer, with official
OAuth API checks preferred, optional cookie support, FFmpeg recording, recording
history, short-file marking, and optional comment capture.

## Features

- Desktop GUI for adding streamers, monitoring status, viewing history, and
  changing settings.
- CLI daemon mode for NAS, VPS, home server, Windows Task Scheduler, or systemd.
- Built-in Web panel for local/server control.
- Official TwitCasting OAuth API first, legacy fallback kept for compatibility.
- Optional `cookies.txt` support for streams that require an authenticated web
  session.
- FFmpeg-based recording with MKV/TS/MP4 modes.
- Worker process isolation for one-streamer-per-process monitoring.
- Sidecar metadata and comment files next to recordings, with a configurable
  comment text template.
- Optional Telegram notifications for recording start, finish, and failure.
- History records, abnormal short-file marking, and per-streamer logs.

## Repository Layout

```text
apps/
  cli/          CLI, daemon, Web panel, Dockerfile
  gui/          Wails desktop app and shared recording core
deploy/
  systemd/      Linux service templates
  windows/      Windows helper scripts
docs/           Architecture, operations, risk notes, release notes
scripts/        Local build and run scripts
download/       Local recording output, ignored by git
```

## Quick Start

### Desktop GUI

Development:

```powershell
.\scripts\run-gui-dev.ps1
```

Build:

```powershell
.\scripts\build-gui.ps1
```

### CLI

Build:

```powershell
.\scripts\build-cli.ps1
```

Check environment:

```powershell
.\apps\cli\twitcasting-recorder.exe doctor --config .\apps\cli\config.example.yaml
```

Monitor with a config file:

```powershell
.\apps\cli\twitcasting-recorder.exe monitor --config .\apps\cli\config.yaml
```

Start the Web panel:

```powershell
.\apps\cli\twitcasting-recorder.exe web --config .\apps\cli\config.yaml --addr 127.0.0.1:8787
```

## Configuration

Start from one of the example files:

- [apps/gui/config.example.yaml](apps/gui/config.example.yaml)
- [apps/cli/config.example.yaml](apps/cli/config.example.yaml)

Local runtime files are intentionally ignored by git:

- `config.yaml`
- `cookies.txt`
- `history.json`
- `download/`
- `logs/`
- generated `*.exe` files
- Go caches and frontend build outputs

See [Data Directories](docs/09-数据目录与发布结构.md) for the intended runtime
layout.

## Web Panel Security

The Web panel can add/remove streamers, start/pause monitoring, and write changes
back to the selected config file. Treat it as an admin panel.

By default it listens on `127.0.0.1`, which is local-only. Do not bind it directly
to the public internet. For remote access, use one of:

- SSH tunnel
- VPN
- reverse proxy with authentication and HTTPS

See [Web Panel Security](docs/10-Web面板安全说明.md).

## API And Legal Notes

This project prefers official TwitCasting OAuth API endpoints for live checks and
comment capture. Some compatibility paths may use web-facing or legacy endpoints
when official data is unavailable. These paths can change without notice and may
be less appropriate for public deployments.

Use this software only for content you are allowed to access and record. Do not
commit OAuth tokens, cookies, private configs, or recorded media.

## Documentation

- [Architecture](docs/01-架构总览.md)
- [Run And Build](docs/02-运行与构建.md)
- [Authentication](docs/03-鉴权与会员内容.md)
- [Data Directories](docs/09-数据目录与发布结构.md)
- [Web Panel Security](docs/10-Web面板安全说明.md)
- [Open Source Checklist](docs/11-GitHub开源发布清单.md)

## License

No license has been selected yet. Before publishing as an open-source repository,
choose and add a license file, such as MIT, Apache-2.0, or GPL-3.0.
