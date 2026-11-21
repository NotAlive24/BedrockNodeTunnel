# Bedrock Node Tunnel — v2.0.0

**Bedrock Node Tunnel** is an open-source client for hosting a vanilla Minecraft Bedrock Dedicated Server behind CGNAT or restrictive routers. It forwards Bedrock TCP and UDP traffic through the Bedrock Node Tunnel relay, with the Raw UDP Fast-Lane and a Yamux fallback.

- **Service:** https://bedrocknode.net
- **Upstream project:** https://github.com/stonercraft420/BedrockNodeTunnel
- **Releases:** https://github.com/NotAlive24/BedrockNodeTunnel/releases

## What's new in v2.0.0

- **Nethernet Desktop Control Suite:** a Go desktop launcher with a browser-based control panel, profiles, engine installation/update, diagnostics, logs and background-service controls. Green is the default accent theme, with five alternative colors that persist across launches; Settings also includes a native uninstall action with optional data removal.
- **Localhost-only UI:** the desktop process binds only to `127.0.0.1` on an automatically selected port. It does not expose the control API to the LAN and uses same-origin requests with a restrictive Content Security Policy.
- **Windows and Linux desktop packaging:** Windows per-user NSIS installer, Debian/Ubuntu package, Arch Linux package, and portable Linux/Windows bundles.
- **Engine update safety:** the desktop updater verifies SHA-256 and the engine's reported version before replacing the working executable, and restores the previous engine if activation or configuration saving fails.
- **CLI reliability and resource limits:** validated node metadata, bounded metadata/token reads, short-write-safe packet framing, bounded raw UDP player sessions and smaller socket buffers.

The v1.3 tunnel capabilities remain: Raw UDP Fast-Lane, mapped TCP streams for multi-server tunnels, connection tuning, and local UDP fallback candidates.

## Release assets

| Platform | CLI | Desktop |
|---|---|---|
| Windows x64 | `nethernet-client-windows-amd64.exe` | `NethernetDesktop-installer.exe`, portable `.zip` |
| Windows ARM64 | `nethernet-client-windows-arm64.exe` | Not included in this desktop release |
| Linux x86_64 | `nethernet-client-linux-amd64`, `.deb`, Arch `.pkg.tar.zst`/PKGBUILD | `.deb`, Arch `.pkg.tar.zst`, portable `.tar.gz` |
| Linux ARM64 | `nethernet-client-linux-arm64`, `.deb`, Arch PKGBUILD/source tarball | Not included in this desktop release |

All assets in the unified `v2.0.0` release are listed in `SHA256SUMS`.

## Linux quick install

There are two separate installers. Choose the one that matches what you want:

### Nethernet Desktop (graphical UI)

On **Arch Linux x86_64** or **Debian/Ubuntu x86_64**, run:

```bash
curl -fsSL https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install-desktop.sh | bash
```

This detects `pacman` or `apt`, downloads the matching native Desktop package from the latest stable release, verifies it against `SHA256SUMS`, and installs it using your package manager. It requires `sudo` for the system package installation. Launch it with `NethernetDesktop`.

The native Desktop packages in v2.0.0 are x86_64-only. The script does not install the CLI separately; the Desktop package provides the graphical launcher.

### CLI only (no graphical UI)

Run:

```bash
curl -fsSL https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.sh | bash
```

The CLI installer detects x86_64/amd64 or ARM64, downloads the matching standalone tunnel-client binary from this repository's latest stable release, verifies its SHA-256 checksum, and installs it to `~/.local/bin/nethernet-client`. It does not require sudo and does not install Nethernet Desktop.

For either one-liner, you can inspect the installer before running it by replacing the pipe with `-o installer.sh` and opening the downloaded file.

The CLI stores its tunnel token and selected node under `~/.config/bedrocknode-tunnel` where possible. On systemd-based distributions, the installer can write a per-user service unit.

To update:

```bash
curl -fsSL https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.sh | bash -s -- update
```

To uninstall but retain saved tunnel data:

```bash
curl -fsSL https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.sh | bash -s -- uninstall
```

To remove saved token and node data as well, explicitly pass `--purge-data`.

## Windows CLI quick install

Run in PowerShell:

```powershell
irm https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.ps1 -OutFile install.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

The installer verifies the downloaded executable against the release's SHA-256 manifest and preserves saved tunnel settings during updates and normal uninstall.

## Nethernet Desktop

The desktop suite is included in the same `v2.0.0` release as the CLI.

**Arch Linux:**

```bash
sudo pacman -U ./nethernet-desktop-2.0.0-1-x86_64.pkg.tar.zst
NethernetDesktop
```

**Debian/Ubuntu:** install the `.deb` package with your package manager, then launch **Nethernet Desktop** from the application menu or run `NethernetDesktop`.

**Windows:** run `NethernetDesktop-installer.exe`. It installs per-user and does not request administrator privileges.

The launcher prints its local URL when started from a terminal and opens the system browser automatically. Keep the launcher running while using the UI. Stop it with Ctrl+C or the **Stop local UI** control in Settings. The control interface is served only from `127.0.0.1`; closing the browser tab does not stop the launcher.

The native desktop packages are x86_64-only in v2.0.0. The desktop app uses the system browser; it does not bundle WebView2, GTK or WebKitGTK. There is no AppImage in this release.

## Security notes

- Desktop HTTP/API access is limited to loopback, exact Host and same-origin requests. It does not listen on `0.0.0.0` or a LAN address.
- Desktop profiles/configuration are stored per-user. CLI token files are written with user-only file permissions where supported.
- **Important transport limitation:** the existing relay wire protocol sends the tunnel token in its TCP authentication handshake and UDP heartbeat without TLS/DTLS encryption. The v2.0.0 client does not change the relay protocol, so this traffic is not protected against an attacker able to observe the network path. End-to-end transport encryption requires a coordinated server-side protocol update. Treat the token as a password and use trusted networks.

## Build from source

CLI:

```bash
go test ./...
go vet ./...
go run ./tools/release -version 2.0.0 -out dist -targets deb,arch,windows
```

Desktop:

```bash
cd desktop
go test ./...
go run ./tools/generate-icon
go build -trimpath -ldflags="-s -w" -o bin/NethernetDesktop .
```

See [desktop packaging documentation](desktop/README-packaging.md) for package details.
