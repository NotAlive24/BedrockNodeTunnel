# Nethernet Desktop

A standalone desktop companion for the Bedrock Node Tunnel client.

## Current status

This is the initial UI foundation. It deliberately does not start, stop, or modify the tunnel client yet. The dashboard labels connection state as **Not connected to client** until the process-management and status protocol are implemented. No simulated connection data is presented as real.

## Development requirements

- Go 1.26+
- Wails v3.0.0
- Linux: GTK4, WebKitGTK 6.0, GCC and pkg-config
- Windows: WebView2 Runtime

On Arch Linux:

```sh
sudo pacman -S --needed base-devel pkgconf gtk4 webkitgtk-6.0
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
cd desktop
go mod download
go run .
```

On Linux, Wails v3 uses GTK4 + WebKitGTK 6.0 by default. See the official Wails installation guide for other distributions and Windows setup.

## Planned integration

The desktop app will manage the existing `nethernet-client` executable as a separate process. The CLI remains independently usable. Background mode will use a Linux systemd **user** service; the UI will not need root privileges to manage it.
