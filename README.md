# Bedrock Node Tunnel — Open-Source Client

**Bedrock Node Tunnel** is a cross-platform Minecraft Bedrock Dedicated Server (\`transport=nethernet\`) TCP/UDP tunneling client. It lets you host a Bedrock server behind CGNAT, Starlink, or restrictive routers without a proxy such as WaterdogPE or Geyser.

- Website and dashboard: https://bedrocknode.net
- Community: https://discord.gg/Fcq2yCjQnV
- Source: https://github.com/NotAlive24/BedrockNodeTunnel

## Supported platforms

| Platform | Architecture | Release asset |
|---|---|---|
| Windows | x64 | \`nethernet-client-windows-amd64.exe\` |
| Windows | ARM64 | \`nethernet-client-windows-arm64.exe\` |
| Linux | x86_64 | \`nethernet-client-linux-amd64\` |
| Linux | ARM64 | \`nethernet-client-linux-arm64\` |
| Debian / Ubuntu | amd64, arm64 | \`.deb\` packages |
| Arch Linux | x86_64, aarch64 | \`PKGBUILD\` + source tarball |

The Go client uses the same networking implementation on Windows and Linux. Release builds are statically linked where supported (\`CGO_ENABLED=0\`).

## Install on Linux

The installer detects Linux and CPU architecture, downloads the matching latest release, verifies its SHA-256 checksum, and installs the binary to \`~/.local/bin\`.

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.sh | bash
\`\`\`

The installer supports install, update, and uninstall:

\`\`\`bash
bash install.sh install
bash install.sh update
bash install.sh uninstall
\`\`\`

To remove the saved tunnel token and region configuration as well:

\`\`\`bash
bash install.sh uninstall --purge-data
\`\`\`

The default installation directory is \`~/.local/bin\`. Override it with \`BNT_INSTALL_DIR=/your/path\`. The installer creates a per-user systemd service when systemd is available; enable it after claiming a tunnel with:

\`\`\`bash
systemctl --user enable --now bedrocknode-tunnel.service
\`\`\`

## Install on Windows

Use PowerShell (Windows 10/11):

\`\`\`powershell
irm https://raw.githubusercontent.com/NotAlive24/BedrockNodeTunnel/main/install.ps1 -OutFile install.ps1
.\\install.ps1
\`\`\`

The PowerShell installer detects x64 or ARM64, downloads the matching \`.exe\`, verifies its SHA-256 checksum, installs it under \`%LOCALAPPDATA%\\Programs\\BedrockNodeTunnel\`, and adds that directory to your user PATH.

Update or uninstall:

\`\`\`powershell
.\\install.ps1 -Action update
.\\install.ps1 -Action uninstall
\`\`\`

To also delete the saved token and node configuration:

\`\`\`powershell
.\\install.ps1 -Action uninstall -PurgeData
\`\`\`

The Bash installer also detects Git Bash/MSYS/Cygwin on Windows and delegates to the PowerShell installer. For native Windows use, PowerShell is recommended.

## First run

1. Start the Bedrock Dedicated Server locally.
2. Run \`nethernet-client\` in a terminal.
3. On first run, the client prints a claim link and opens it in your browser when a desktop session is available.
4. Claim the tunnel in the Bedrock Node Tunnel dashboard.
5. Copy the connection settings shown by the dashboard into your BDS \`server.properties\`.

The client stores its token and selected region in the user's configuration directory. Updating or uninstalling the binary does not remove this data unless you explicitly request a purge.

## Useful options

\`\`\`text
-version                 Print the client version
-node <US|BR|ASIA|host>  Select a region or custom node
-control <host:port>     Set the relay control address
-web <URL>               Set the dashboard URL
-token <value>           Use a supplied tunnel token
-reset-token             Forget the saved token and claim a new tunnel
-local-http <host:port>  Local Bedrock TCP target (default 127.0.0.1:19132)
-local-udp-ip <IP>       Local IP for Bedrock UDP (auto-detected when empty)
-local-udp-port <port>   Fallback local UDP port (default 47334)
\`\`\`

## Build from source

Requires Go 1.21 or newer:

\`\`\`bash
go test ./...
go run .
\`\`\`

Build a single target manually:

\`\`\`bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o nethernet-client.exe .
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o nethernet-client .
\`\`\`

Build all release artifacts (Windows x64/ARM64, Linux x86_64/ARM64, Debian packages, and Arch packaging):

\`\`\`bash
go run ./tools/release -version 1.2 -out dist -targets deb,arch,linux,windows
\`\`\`

GitHub Actions runs tests and cross-compilation checks on pushes and pull requests. Pushing a tag such as \`v1.2\` builds and publishes the release assets automatically.

## Security and scope

The client forwards traffic between the Bedrock Node Tunnel relay and the local Bedrock server. It does not modify the Minecraft world or server files. The tunnel token is stored in the user's configuration directory; keep that file private.
