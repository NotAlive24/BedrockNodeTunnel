# ⚡ Bedrock Node Tunnel — Open-Source Client

![Bedrock Node Tunnel](logo.jpg)

**Bedrock Node Tunnel** is a 100% Vanilla Minecraft Bedrock Dedicated Server (`transport=nethernet`) WebRTC UDP + TCP tunneling client with a **Raw UDP Fast-Lane**. Host your Bedrock server behind Starlink, CGNAT, or strict routers without WaterdogPE, Geyser, or wiped player XUIDs/inventories.

- 🌐 **Get Your Free Tunnel & Ports:** [https://bedrocknode.net](https://bedrocknode.net)
- 💬 **Join Our Discord:** [https://discord.gg/Fcq2yCjQnV](https://discord.gg/Fcq2yCjQnV)
- 💻 **Official Repository:** [https://github.com/stonercraft420/BedrockNodeTunnel](https://github.com/stonercraft420/BedrockNodeTunnel)

---

## 🔍 Why Is This Repository Open-Source?
We believe server hosts should never have to run mystery `.exe` files on their machines. This repository contains the complete source code (`client.go`) for `nethernet-client.exe`:
- **Zero File or System Access:** Only forwards traffic between the Bedrock Node Tunnel regional hubs (`US`, `TX`, `UK`, `BR`, `ASIA`, `OC`) and your local Bedrock server (`127.0.0.1:19132` + local WebRTC UDP ports).
- **Single Official Dependency:** Uses only the standard Go library and `github.com/hashicorp/yamux`. Release builds are statically linked (`CGO_ENABLED=0`).

## Supported Platforms

| Platform | Architecture | Release Asset |
|---|---|---|
| Windows | x64 | `nethernet-client-windows-amd64.exe` |
| Windows | ARM64 | `nethernet-client-windows-arm64.exe` |
| Linux | x86_64 | `nethernet-client-linux-amd64` |
| Linux | ARM64 | `nethernet-client-linux-arm64` |
| Debian / Ubuntu | amd64, arm64 | `.deb` packages |
| Arch Linux | x86_64, aarch64 | `PKGBUILD` + source tarball |

---

## 🚀 Option 1: Windows Quick Start (Pre-Built `.exe` or PowerShell Installer)

### Method A: Direct `.exe` Download
1. Download `nethernet-client.exe` from [https://bedrocknode.net](https://bedrocknode.net) (or the [Releases](../../releases) tab).
2. Double-click `nethernet-client.exe`.
   * *Note: If Windows SmartScreen says "Windows protected your PC", click **More info** -> **Run anyway**.*
3. Click the Claim Link shown in the console (or enter the 6-digit code) to link the tunnel to your free account at `bedrocknode.net`.
4. Copy the 4 `server.properties` lines shown on your dashboard into your Bedrock Dedicated Server and restart `bedrock_server.exe`.

### Method B: One-Line PowerShell Installer (Windows 10/11)
```powershell
irm [https://raw.githubusercontent.com/stonercraft420/BedrockNodeTunnel/main/install.ps1](https://raw.githubusercontent.com/stonercraft420/BedrockNodeTunnel/main/install.ps1) -OutFile install.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
