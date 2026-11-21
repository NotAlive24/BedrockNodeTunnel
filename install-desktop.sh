#!/usr/bin/env bash
set -Eeuo pipefail

REPO="NotAlive24/BedrockNodeTunnel"
API_URL="https://api.github.com/repos/${REPO}/releases/latest"
RELEASE_BASE="https://github.com/${REPO}/releases/download"
TMP_DIR=""

cleanup() {
  if [[ -n "$TMP_DIR" && -d "$TMP_DIR" ]]; then
    rm -rf -- "$TMP_DIR"
  fi
}
trap cleanup EXIT

fail() {
  echo "Nethernet Desktop installer: $*" >&2
  exit 1
}

download() {
  local url="$1"
  local output="$2"

  case "$url" in
    https://*) ;;
    *) fail "refusing non-HTTPS URL: $url" ;;
  esac

  if command -v curl >/dev/null 2>&1; then
    curl --proto '=https' --proto-redir '=https' \
      --fail --location --silent --show-error \
      --retry 3 --connect-timeout 15 --max-time 120 \
      -H "User-Agent: Nethernet-Desktop-Installer" \
      "$url" --output "$output"
  elif command -v wget >/dev/null 2>&1; then
    wget --https-only --max-redirect=5 --timeout=15 --tries=3 \
      --user-agent="Nethernet-Desktop-Installer" \
      --output-document="$output" "$url"
  else
    fail "curl or wget is required."
  fi
}

if [[ "$(uname -s)" != "Linux" ]]; then
  fail "this installer supports Linux. On Windows, use NethernetDesktop-installer.exe."
fi

case "$(uname -m)" in
  x86_64|amd64) ;;
  *)
    fail "the native Desktop packages currently support x86_64 only."
    ;;
esac

if [[ -f /etc/arch-release ]] && command -v pacman >/dev/null 2>&1; then
  PACKAGE_MANAGER="pacman"
elif [[ -f /etc/debian_version ]] && command -v apt-get >/dev/null 2>&1; then
  PACKAGE_MANAGER="apt"
else
  fail "supported package managers are pacman (Arch) and apt (Debian/Ubuntu)."
fi

if (( EUID != 0 )) && ! command -v sudo >/dev/null 2>&1; then
  fail "sudo is required to install a system package. Run as root or install sudo."
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/nethernet-desktop.XXXXXXXX")"
RELEASE_JSON="$TMP_DIR/release.json"
CHECKSUMS="$TMP_DIR/SHA256SUMS"

echo "Checking the latest stable Nethernet Desktop release..."
download "$API_URL" "$RELEASE_JSON"

VERSION="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"v\([^"]*\)".*/\1/p' "$RELEASE_JSON" | head -n 1)"
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  fail "could not determine a valid stable release version from GitHub."
fi

case "$PACKAGE_MANAGER" in
  pacman)
    ASSET="nethernet-desktop-${VERSION}-1-x86_64.pkg.tar.zst"
    ;;
  apt)
    ASSET="nethernet-desktop_${VERSION}-1_amd64.deb"
    ;;
esac

BASE_URL="${RELEASE_BASE}/v${VERSION}"
PACKAGE="$TMP_DIR/$ASSET"

echo "Downloading Nethernet Desktop v${VERSION} for Linux x86_64..."
download "$BASE_URL/$ASSET" "$PACKAGE"
download "$BASE_URL/SHA256SUMS" "$CHECKSUMS"

EXPECTED="$(awk -v name="$ASSET" '$2 == name || $2 == "*" name { print $1; exit }' "$CHECKSUMS")"
[[ "$EXPECTED" =~ ^[A-Fa-f0-9]{64}$ ]] || fail "the release checksum manifest has no valid entry for $ASSET."

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "$PACKAGE" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL="$(shasum -a 256 "$PACKAGE" | awk '{print $1}')"
else
  fail "sha256sum or shasum is required to verify the package."
fi

EXPECTED="$(printf '%s' "$EXPECTED" | tr 'A-F' 'a-f')"
ACTUAL="$(printf '%s' "$ACTUAL" | tr 'A-F' 'a-f')"
[[ "$ACTUAL" == "$EXPECTED" ]] || fail "SHA-256 verification failed for $ASSET."

echo "Checksum verified."
case "$PACKAGE_MANAGER" in
  pacman)
    if (( EUID == 0 )); then
      pacman -U "$PACKAGE"
    else
      sudo pacman -U "$PACKAGE"
    fi
    ;;
  apt)
    if (( EUID == 0 )); then
      apt-get install "$PACKAGE"
    else
      sudo apt-get install "$PACKAGE"
    fi
    ;;
esac

echo
echo "Nethernet Desktop v${VERSION} is installed."
echo "Launch it with: NethernetDesktop"
