# ⚡ Bedrock Node Tunnel — Open-Source Client

![Bedrock Node Tunnel](logo.jpg)

**Bedrock Node Tunnel** is a 100% Vanilla Minecraft Bedrock Dedicated Server (`transport=nethernet`) WebRTC UDP + TCP tunneling client with a **Raw UDP Fast-Lane**. Host your Bedrock server behind Starlink, CGNAT, or strict routers without WaterdogPE, Geyser, or wiped player XUIDs/inventories.

- 🌐 **Get Your Free Tunnel & Ports:** [https://bedrocknode.net](https://bedrocknode.net)
- 💬 **Join Our Discord:** [https://discord.gg/Fcq2yCjQnV](https://discord.gg/Fcq2yCjQnV)
- 💻 **Upstream Project:** [https://github.com/stonercraft420/BedrockNodeTunnel](https://github.com/stonercraft420/BedrockNodeTunnel)

---

## 🔍 Why Is This Repository Open-Source?
We believe server hosts should never have to run mystery `.exe` files on their machines. This repository contains the complete source code (`client.go`) for `nethernet-client.exe`:
- **Zero File or System Access:** Only forwards traffic between the Bedrock Node Tunnel regional hubs (`US`, `TX`, `UK`, `BR`, `ASIA`, `OC`) and your local Bedrock server (`127.0.0.1:19132` + local WebRTC UDP ports).
- **Single Official Dependency:** Uses only the standard Go library and `github.com/hashicorp/yamux`. Release builds are statically linked (`CGO_ENABLED=0`).

## Supported Platforms

## What's new in v1.3

- **Raw UDP Fast-Lane:** carries Bedrock player UDP traffic over the raw UDP fast path.
- **Multi-server TCP mapping:** supports mapped TCP streams that identify the target local TCP port.
- **Network tuning:** configures TCP no-delay/keepalive and larger TCP/UDP socket buffers to improve tunnel throughput.
- **Local UDP fallback candidates:** tries the configured LAN address and IPv4/IPv6 loopback targets.


| Platform | Architecture | Release Asset |
|---|---|---|
| Windows | x64 | `nethernet-client-windows-amd64.exe` |
| Windows | ARM64 | `nethernet-client-windows-arm64.exe` |
| Linux | x86_64 | `nethernet-client-linux-amd64` |
| Linux | ARM64 | `nethernet-client-linux-arm64` |
| Debian / Ubuntu | amd64, arm64 | `.deb` packages |
| Arch Linux | x86_64, aarch64 | `PKGBUILD` + source tarball |

---

## 🐧 Option 1: Linux Quick Install (Ubuntu, Debian, Arch and other distros)

Run this one-line command in a terminal:

```bash
curl -fsSL https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.sh | bash
```

The installer detects x86_64/amd64 or ARM64, downloads the matching latest Linux release from **this repository**, verifies it against `SHA256SUMS`, and installs it to `~/.local/bin/nethernet-client`. It does not require `sudo`.

The installer preserves saved tunnel settings in `~/.config/bedrocknode-tunnel`. On systemd-based distributions, it also writes a per-user service unit; enable it with:

```bash
systemctl --user enable --now bedrocknode-tunnel.service
```

To update or uninstall, use:

```bash
curl -fsSL https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.sh | bash -s -- update
curl -fsSL https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.sh | bash -s -- uninstall
```

To remove the saved token and configuration as well, explicitly pass `--purge-data`:

```bash
curl -fsSL https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.sh | bash -s -- uninstall --purge-data
```

You can also use the Debian package attached to the [v1.3 release](https://github.com/NotAlive24/BedrockNodeTunnel/releases/tag/v1.3). From the directory containing the downloaded package, run `sudo apt install ./bedrocknode-tunnel_1.3_amd64.deb` (use the ARM64 package on ARM64 systems).

---

## 🪟 Option 2: Windows Quick Start (Pre-Built `.exe` or PowerShell Installer)

### Method A: Direct `.exe` Download
1. Download `nethernet-client-windows-amd64.exe` (or `nethernet-client-windows-arm64.exe`) from the [Releases](https://github.com/NotAlive24/BedrockNodeTunnel/releases) tab.
2. Run the executable.
3. Open the Claim Link shown in the console (or enter the 6-digit code) to link the tunnel to your free account at `bedrocknode.net`.
4. Copy the `server.properties` lines shown on your dashboard into your Bedrock Dedicated Server and restart it.

### Method B: PowerShell Installer (Windows 10/11)

Download and run the installer from PowerShell:

```powershell
irm https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.ps1 -OutFile install.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\\install.ps1
```

The installer verifies the downloaded executable using the release's SHA-256 manifest and preserves saved tunnel settings during updates and uninstall unless `-PurgeData` is explicitly supplied.
