# Privacy Notes

TwitCasting Recorder stores runtime data locally unless you explicitly place the
project in a synchronized folder or deploy it to a remote server.

## Local Data

The application may create or read:

- `config.yaml`: local settings, streamer list, OAuth fields
- `cookies.txt`: optional browser cookies
- `history.json`: local recording history
- `download/`: recorded media and sidecar text/jsonl files
- `logs/`: local diagnostic logs

## Network Requests

The recorder connects to TwitCasting APIs and stream URLs to check live status,
fetch comments, and record media. If proxy settings are enabled, these requests
are routed through the configured proxy.

## User Responsibility

Do not publish configs, cookies, OAuth tokens, private logs, or recorded media
without permission. Recorded streams and comments may contain personal data from
streamers and viewers.
