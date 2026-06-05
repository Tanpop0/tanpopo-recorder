# TwitCasting Recorder CLI

Server-friendly entrypoint for long-running recording. The GUI and CLI share the
same core packages for live checking, recording, comments, and history.

## Commands

```powershell
twitcasting-recorder doctor --config config.yaml
twitcasting-recorder monitor --config config.yaml
twitcasting-recorder web --config config.yaml --addr 127.0.0.1:8787
twitcasting-recorder record --config config.yaml c:example
twitcasting-recorder auth verify --config config.yaml
```

No command defaults to `monitor`, so the old `go run . --config config.yaml`
style still starts monitoring.

## Config

Copy `config.example.yaml` to `config.yaml`, then add streamers and auth. Prefer
OAuth for official API checks and comment capture. `cookies.txt` is optional and
should stay local.

`recording.comment_text_template` controls the human-readable comment txt line
format. The JSONL sidecar keeps structured fields for future replay/player work.

`notifications.telegram` can send start, finish, and failure pushes through a
Telegram bot. Keep bot tokens out of git.

`schedule` is optional per streamer. When omitted, the CLI uses
`@every <check_interval_seconds>s`.

## Web Panel

```powershell
twitcasting-recorder web --config config.yaml --addr 127.0.0.1:8787
```

The built-in panel can add/remove streamers, start/pause monitoring, and show
recent status logs. It writes streamer changes back to the selected config file.

The default address only listens on localhost. If you bind to `0.0.0.0`, put it
behind a trusted reverse proxy, VPN, or SSH tunnel first.

## Worker Mode

Set `recording.worker_enabled: true` to use one isolated `recorder-worker`
process per monitored streamer. Put `recorder-worker.exe` beside the CLI binary,
or set `recording.worker_path`.

## Server Notes

Use `mkv` or `ts` for long-running recording. MP4 is supported through a
post-record remux step, but interruption-safe recording is easier with `mkv` or
`ts`.
