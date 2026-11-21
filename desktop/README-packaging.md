# Nethernet Desktop packages — v2.0.0

The desktop control suite is a standalone Go launcher that serves its embedded browser UI on an ephemeral `127.0.0.1` port. It uses the user's system browser and has no embedded WebView runtime dependency.

## Linux

The unified `v2.0.0` release produces:
- Debian/Ubuntu amd64 `.deb`
- Arch Linux x86_64 `.pkg.tar.zst`
- Portable Linux x86_64 `.tar.gz` containing the launcher and matching CLI engine

The native packages install the launcher in `/usr/bin`, the generated application icon in the hicolor icon theme, and a desktop entry. They do not require GTK4 or WebKitGTK at runtime. The in-app uninstall action uses `pkexec` with the system package manager and preserves user data unless the user explicitly selects purge.

## Windows

The unified release creates:
- Per-user NSIS installer for x64
- Portable x64 `.zip` containing the launcher and matching CLI engine

The installer does not request administrator privileges, and does not bundle or require WebView2. The application includes an in-app uninstall action that hands off to the NSIS uninstaller after closing the running desktop process. User data is preserved unless the purge option is selected.

## Unified release

A single `v2.0.0` tag triggers the root release workflow. It builds the CLI assets, Linux desktop packages, Windows desktop installer and portable desktop bundles, combines them into one GitHub release, and generates one SHA-256 manifest. The desktop-only workflows are manual artifact builders and do not publish separate GitHub releases.

There is no AppImage in v2.0.0: the previous Wails AppImage packager required GTK detection and failed after the app moved to a browser-based launcher. DEB, Arch and portable formats are supported instead.

## Local build

Requirements: Go 1.26 or newer. NSIS is additionally required to build the Windows installer.

From this directory:

```sh
go test ./...
go run ./tools/generate-icon
go build -trimpath -ldflags="-s -w" -o bin/NethernetDesktop .
```

For Linux packages, install nfpm and run it with `build/linux/nfpm/nfpm.yaml` from this directory. For Windows, build `bin/NethernetDesktop.exe`, then run NSIS against `build/windows/nsis/project.nsi` from the NSIS script directory, defining `ARG_DESKTOP_BINARY` as the absolute path to the executable. The version can be supplied with `PRODUCT_VERSION`.
