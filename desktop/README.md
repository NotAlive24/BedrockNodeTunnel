# Nethernet Desktop Control Suite

Nethernet Desktop is a small Go launcher and local browser control panel for the Bedrock Node Tunnel client. It does not embed a browser engine.

## Localhost-only design

When started, the launcher:
- Listens on an OS-selected free port bound specifically to IPv4 loopback `127.0.0.1`.
- Opens the UI in the user's default browser.
- Serves the UI and its JSON API from the same origin.
- Rejects non-loopback peer addresses, unexpected Host headers, and foreign Origin headers.
- Does not enable CORS and does not bind to LAN interfaces such as `0.0.0.0` or a machine's LAN IP.

The terminal prints the exact local URL. Keep the launcher process open while using the UI. Press Ctrl+C in that terminal, or use **Stop local UI** in Settings, to close the local server. The local API is not intended to be a network service or to be exposed through a reverse proxy.

## Features

- Start, stop and restart the tunnel engine as a managed foreground process.
- Save, edit, switch and delete connection profiles.
- Install/update the engine from this fork's latest GitHub release with SHA-256 verification.
- Inspect engine version and executable path.
- Run local TCP, relay TCP and service API diagnostics.
- View a bounded live log buffer with token-like values redacted.
- Linux systemd user service and Windows per-user Task Scheduler background mode.
- Per-user configuration; normal operation does not require administrator privileges.

## Build and test

Requirements: Go 1.26 or newer. For Windows installer packaging, NSIS is also required. A modern system browser is required at runtime.

```sh
cd desktop
go test ./...
go run ./tools/generate-icon
go build -trimpath -ldflags="-s -w" -o bin/NethernetDesktop .
```

The Linux DEB and Arch packages are built with [nfpm](https://nfpm.goreleaser.com/). The Windows per-user installer is built with NSIS. Neither package build requires Wails, WebView2 or WebKitGTK.

## Configuration

Desktop settings and profiles are stored in the user's config directory under `bedrocknode-tunnel/desktop.json`. The tunnel engine's own token/config files remain managed by the existing client; this app does not read or print those secrets.

The engine updater installs into `bedrocknode-tunnel/bin/`. Linux background mode creates a per-user systemd unit; Windows background mode uses a per-user Task Scheduler task. Closing the browser does not stop the launcher process or a background-managed tunnel; stop the launcher separately with Ctrl+C or the Settings control.

## Diagnostics caveats

TCP reachability checks do not prove the authenticated raw UDP fast-lane works. The app never reports a successful public tunnel solely because a local TCP port is reachable.
