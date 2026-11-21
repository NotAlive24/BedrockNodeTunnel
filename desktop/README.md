# Nethernet Desktop Control Suite v2.0.0

Nethernet Desktop is a small Go launcher and local browser control panel for the Bedrock Node Tunnel client. It uses the system browser and does not embed a browser engine.

## Localhost-only design

When started, the launcher:
- Listens on an OS-selected free port bound specifically to IPv4 loopback `127.0.0.1`.
- Opens the UI in the user's default browser.
- Serves the UI and JSON API from the same origin.
- Rejects non-loopback peers, unexpected Host headers and foreign Origin headers.
- Does not enable CORS or bind to a LAN interface.
- Applies a Content Security Policy that only permits scripts from the local origin.

The terminal prints the exact local URL. Keep the launcher process open while using the UI. Press Ctrl+C or use **Stop local UI** in Settings to close the server. Closing the browser tab does not stop the launcher. Do not expose this local control server through a reverse proxy or port-forward.

## Features

- Start, stop and restart the tunnel engine as a managed foreground process.
- Save, edit, switch and delete connection profiles.
- Install/update the engine from this fork's latest stable CLI release with SHA-256 and version verification.
- Preserve the previous engine if a downloaded replacement fails validation or cannot be activated.
- Inspect engine version and executable path without launching a version-check process on every UI refresh.
- Run local TCP, relay TCP and service API diagnostics.
- View a bounded live log buffer with token-like values and one-time claim codes redacted.
- Linux systemd user service and Windows per-user Task Scheduler background mode.
- Per-user configuration; normal operation does not require administrator privileges.

## Build and test

Requirements: Go 1.26 or newer. NSIS is required only to build the Windows installer. A modern system browser is required at runtime.

```sh
cd desktop
go test ./...
go run ./tools/generate-icon
go build -trimpath -ldflags="-s -w" -o bin/NethernetDesktop .
```

Linux DEB/Arch packages use nfpm. The Windows per-user installer uses NSIS. The runtime app does not require Wails, WebView2, GTK or WebKitGTK.

## Configuration

Desktop settings and profiles are stored under the user's config directory in `bedrocknode-tunnel/desktop.json` (normally `~/.config/bedrocknode-tunnel/desktop.json` on Linux). The CLI's tunnel token/config files remain managed by the CLI; the desktop app does not read or display token contents.

The engine updater installs into `bedrocknode-tunnel/bin/`. Linux background mode creates a per-user systemd unit; Windows background mode uses a per-user Task Scheduler task. Background mode is disabled when the required service tooling is unavailable.

## Security limitation

The desktop HTTP control API is loopback-only. However, the existing relay wire protocol transmits the tunnel token in its TCP authentication handshake and UDP heartbeat without TLS/DTLS. This desktop release does not change that server protocol. Use trusted networks and treat the token as a password; end-to-end transport encryption requires a coordinated server-side protocol update.

## Diagnostics caveats

TCP reachability checks do not prove the authenticated raw UDP fast-lane works. The app never reports a successful public tunnel solely because a local TCP port is reachable.
