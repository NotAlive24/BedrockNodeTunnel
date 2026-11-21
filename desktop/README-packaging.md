# Nethernet Desktop packages

The desktop control suite is a standalone Go launcher that serves its embedded browser UI on an ephemeral `127.0.0.1` port. It uses the user's system browser and has no embedded WebView runtime dependency.

## Linux

The native package workflow produces:
- Debian/Ubuntu `.deb`
- Arch Linux `.pkg.tar.zst`

The packages install the launcher in `/usr/bin`, the generated application icon in the hicolor icon theme, and a desktop entry. No GTK4 or WebKitGTK runtime dependency is required by the control suite.

## Windows

The native workflow creates a per-user NSIS installer. It does not request administrator privileges and does not bundle or require WebView2. It creates Start Menu and Desktop shortcuts.

## Portable archives

The separate desktop archive workflow includes the launcher and matching `nethernet-client` binary:
- `desktop-v*` tags publish portable Linux `.tar.gz` and Windows `.zip` archives.
- `desktop-native-v*` tags publish the native Linux packages and Windows NSIS installer.

These tag prefixes are deliberately separate so two workflows do not race to publish the same GitHub release.

## Local build

Requirements: Go 1.26 or newer. NSIS is additionally required to build the Windows installer.

From this directory:

```sh
go test ./...
go run ./tools/generate-icon
go build -trimpath -ldflags="-s -w" -o bin/NethernetDesktop .
```

For Linux package files, install nfpm and run it with `build/linux/nfpm/nfpm.yaml` from this directory. For Windows, build `bin/NethernetDesktop.exe`, then run NSIS against `build/windows/nsis/project.nsi` from the NSIS script directory, defining `ARG_DESKTOP_BINARY` as the absolute path to the executable.
