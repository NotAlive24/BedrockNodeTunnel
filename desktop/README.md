# Nethernet Desktop Suite

A local browser-based control centre for the Bedrock Node Tunnel client. The launcher binds only to 127.0.0.1 on an available port and opens the UI in the default browser. The CLI remains a separate, independently usable program. The CLI remains a separate, independently usable program.

## Features

- Start, stop and restart the existing tunnel engine as a managed process.
- Save multiple connection profiles and switch between them.
- Install or update the engine from this fork's latest GitHub release with SHA-256 verification.
- Inspect engine version and path.
- Run local TCP, relay TCP and service API diagnostics.
- View a bounded live log buffer with token-like values redacted.
- Linux systemd user service and Windows per-user Task Scheduler background mode.
- User-scoped configuration; normal operation does not require administrator privileges.

## Development

Requirements: Go 1.26+, Wails CLI v3.0.0 for packaging helpers, and platform packaging tools. The running UI uses the system browser; it does not embed WebKit or require WebView2.

    go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
    cd desktop
    go mod download
    wails3 generate bindings
    go test ./...
    go run .

The frontend communicates with the local Go process through a same-origin JSON API.

## Configuration

Desktop settings and profiles are stored in the user's config directory under bedrocknode-tunnel/desktop.json. The tunnel engine's own token/config files remain managed by the existing client; this app does not read or print those secrets.

Linux background mode writes a per-user systemd unit. Windows background mode uses a per-user Task Scheduler task. Closing the UI leaves a background-managed tunnel running; foreground-managed tunnels are stopped when the app exits.

## Diagnostics caveats

TCP reachability checks do not prove the authenticated raw UDP fast-lane works. The app never reports a successful public tunnel solely because a local TCP port is reachable.
