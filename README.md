# ⚡ Bedrock Node Tunnel (Open-Source Client)

![Bedrock Node Tunnel](logo.jpg)

**Bedrock Node Tunnel** is a 100% Vanilla Minecraft Bedrock Dedicated Server (`transport=nethernet`) WebRTC UDP + TCP tunneling client. Host your Bedrock server behind Starlink, CGNAT, or strict routers without WaterdogPE, Geyser, or wiped player XUIDs/inventories.

🌐 **Get Your Free Tunnel & Ports:** [https://bedrocknode.net](https://bedrocknode.net)  
💬 **Join Our Discord:** [https://discord.gg/Fcq2yCjQnV](https://discord.gg/Fcq2yCjQnV)

---

## 🔍 Why Is This Repository Open-Source?
We believe server hosts should never have to run mystery `.exe` files on their machines. This repository contains the complete source code (`client.go`) for `nethernet-client.exe`:
* **Zero File or System Access:** Only forwards traffic between the Bedrock Node Tunnel relay (`51.222.142.179:7000`) and your local Bedrock server (`127.0.0.1:19132` + local WebRTC UDP ports).
* **Single Official Dependency:** Uses only the standard Go library and `github.com/hashicorp/yamux`.

---

## 🚀 Option 1: Quick Start (Pre-Built Windows `.exe`)
1. Download `nethernet-client.exe` from [https://bedrocknode.net](https://bedrocknode.net) (or the Releases tab).
2. Double-click `nethernet-client.exe`.
   * *Note: If Windows SmartScreen says "Windows protected your PC", click **More info** -> **Run anyway**.*
3. Click the Claim Link shown in the console to link the tunnel to your free account at `bedrocknode.net`.
4. Copy the 4 `server.properties` lines shown on your dashboard into your Bedrock Dedicated Server and restart `bedrock_server.exe`.

---

## 🛠️ Option 2: Build or Run From Source Yourself (No `.exe` Download Needed)
If you prefer to compile the client yourself or avoid Windows Defender `.exe` warnings:

1. Install [Go for Windows](https://go.dev/dl/).
2. Download or clone this repository (`client.go`, `go.mod`, `go.sum`).
3. Open PowerShell in the folder and run:
   ```powershell
   go run client.go
   ```

---

## 📦 Prebuilt releases (Debian/Ubuntu and Arch Linux)

Download the files from the [Releases](../../releases) page. On the first run the client prints a **Claim Link** (and opens it in your browser when a desktop is available). The tunnel token is saved automatically in `~/.config/bedrocknode-tunnel/`.

**Debian / Ubuntu / Raspberry Pi OS:**
```bash
sudo apt install ./bedrocknode-tunnel_1.0_amd64.deb    # use the arm64 file on ARM
nethernet-client                                       # first run: claim your tunnel
systemctl --user enable --now bedrocknode-tunnel       # keep it running in the background
loginctl enable-linger "$USER"                         # start at boot without logging in
```

**Arch Linux:** download `PKGBUILD` and `bedrocknode-tunnel-1.0.tar.gz` into one folder, then:
```bash
makepkg -si
```

Useful flags: `-version` prints the version, `-reset-token` forgets the saved token and claims a new one, `-token <value>` skips the claim step. See `man nethernet-client` for all options.

Windows users: use Option 1 or 2 above for now.
