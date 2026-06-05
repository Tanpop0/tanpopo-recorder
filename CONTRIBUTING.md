# Contributing

Thanks for taking a look at the project.

## Development Setup

Requirements:

- Go
- Node.js and npm
- FFmpeg and FFprobe
- Wails CLI for GUI development

Useful commands:

```powershell
.\scripts\build-cli.ps1
.\scripts\build-gui.ps1
```

Run checks:

```powershell
go test ./apps/cli/...
go test ./apps/gui/...
```

## Runtime Data

Do not commit local runtime data:

- `config.yaml`
- `cookies.txt`
- `history.json`
- `download/`
- `logs/`
- generated binaries

Use the example config files when documenting or testing default behavior.

## Pull Request Expectations

- Keep recording stability changes small and testable.
- Prefer official TwitCasting API paths where possible.
- Document any new config key in the example config and docs.
- Avoid logging OAuth tokens, cookie values, or full private URLs.
