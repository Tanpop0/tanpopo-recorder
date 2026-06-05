# Security Policy

## Supported Scope

Security reports should focus on the current GUI, CLI, worker process, Web panel,
configuration handling, logging, and recording workflow.

## Sensitive Data

The following files can contain private information and must not be committed or
shared:

- `config.yaml`
- `cookies.txt`
- `history.json`
- `download/`
- `logs/`

OAuth access tokens and browser cookies grant access to user accounts or private
stream sessions. Treat them like passwords.

## Web Panel

The Web panel is an admin interface. Anyone who can access it can add or remove
streamers, start or pause monitoring, and modify the selected config file.

Default usage should bind to localhost:

```powershell
twitcasting-recorder web --config config.yaml --addr 127.0.0.1:8787
```

For remote access, use SSH tunneling, VPN, or a reverse proxy with authentication
and HTTPS. Do not expose the panel directly to the public internet.

## Reporting

If this repository is published publicly, add a private contact method here, such
as a GitHub Security Advisory link or a security email address.
