# Nethernet Desktop

A standalone desktop companion for the Bedrock Node Tunnel client. The desktop app manages the existing `nethernet-client` executable; it does not reimplement the tunnel protocol.

## Current implementation

- Reads and saves a desktop connection profile in the user's configuration directory (`bedrocknode-tunnel/desktop.json`).
- Starts and stops the existing CLI as a child process and streams its output into the UI.
- Provides a cross-platform system tray menu to reopen the dashboard, start/stop the foreground client, and quit.
- By default, closing the dashboard hides it to the tray; this can be disabled in Settings. Quitting explicitly exits the UI and stops only its foreground-managed client; a separately installed systemd background service remains independent.
- Reports `connected` only after the client logs successful relay authentication. A process merely being alive is not treated as a confirmed tunnel.
- Redacts token-like values from displayed and copied logs. The desktop profile does not store the tunnel token; the existing CLI token file remains authoritative.
- On Linux, installs and manages a separate user-level systemd unit named `nethernet-desktop-tunnel.service`. It does not overwrite the existing `bedrocknode-tunnel.service`.
- Detects an active or enabled legacy `bedrocknode-tunnel.service` and refuses to start a duplicate tunnel client.
- Keeps the original CLI and root Go module untouched.

The UI currently supports one saved connection profile. Named profile switching, Windows tunnel-service management, richer diagnostics and in-app updates are later milestones.

## Development requirements

- Go 1.26+
- Wails v3.0.0
- Linux: GTK4, WebKitGTK 6.0, GCC and pkg-config
- Windows: WebView2 Runtime and the Wails Windows build toolchain

On Arch Linux:

```sh
sudo pacman -S --needed base-devel pkgconf gtk4 webkitgtk-6.0
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
cd desktop
go mod tidy
wails3 generate bindings
go run .
```

On Linux, Wails v3 uses GTK4 + WebKitGTK 6.0 by default. Run `wails3 doctor` if the native toolchain is not detected. See the official Wails installation guide for other distributions and Windows setup.

## Client integration

Install `nethernet-client` separately using the existing repository installer. Leave the executable path blank in Connection settings to discover it in common install locations or through `PATH`, or set an explicit path. The desktop app invokes the existing client directly with the saved connection flags; it does not use a shell.

The profile is saved separately from the CLI's tunnel token and selected-node files. Saving settings never deletes or replaces the tunnel token.

## Linux background mode

Use **Settings → Background tunnel service → Install & enable**. This creates a systemd user unit under:

```text
$XDG_CONFIG_HOME/systemd/user/nethernet-desktop-tunnel.service
```

(or `~/.config/systemd/user/` when `XDG_CONFIG_HOME` is not set). It runs the existing client directly, without the desktop UI. It restarts on failure and uses the selected saved connection settings. After changing settings, restart the background service from the UI so the running process uses the new arguments.

The service is user-scoped and does not require root. It runs `nethernet-client` directly, without launching the Wails UI, and starts at login once enabled. The app checks for the older `bedrocknode-tunnel.service` before starting another client; stop and disable that older service first if it is active or enabled. **Remove service** only removes the desktop-managed unit; it does not uninstall the CLI, delete the profile or remove the tunnel token.

## Desktop autostart

Settings → Desktop startup registers the graphical app at login. Linux uses the user's XDG autostart directory; Windows uses the current user's Run registry key. **Start minimized to the system tray** adds the `--background` argument, starting the tray process without showing the dashboard. This is separate from the Linux background tunnel service above: use that service when you want only the tunnel process running with no desktop UI.

For manual tray-only launch, start the app with `--background`. A normal launch always opens the dashboard.

## Checks

From this directory:

```sh
go test ./...
go vet ./backend
wails3 generate bindings
go build .
```
