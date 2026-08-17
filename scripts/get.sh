#!/bin/sh
set -eu

REPO="${DA_REPO:-ngtrthanh/deploy-agent}"
VERSION="${DA_VERSION:-edge}"
DEST="${DA_DEST:-./deploy-agent}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)

case "$os" in
  linux|darwin) ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac

case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  armv7l|armv7*) arch=armv7 ;;
  armv6l|armv6*) arch=armv6 ;;
  i386|i486|i586|i686) arch=386 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

case "$os/$arch" in
  darwin/armv6|darwin/armv7|darwin/386) echo "unsupported platform: $os/$arch" >&2; exit 1 ;;
esac

asset="deploy-agent-${os}-${arch}"
base="https://github.com/${REPO}/releases/download/${VERSION}"
tmp="${DEST}.tmp.$$"
checksums="${DEST}.checksums.$$"
trap 'rm -f "$tmp" "$checksums"' EXIT INT TERM

fetch() {
  url=$1
  out=$2
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --connect-timeout 10 -o "$out" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$out" "$url"
  else
    echo "curl or wget is required" >&2
    exit 1
  fi
}

fetch "${base}/${asset}" "$tmp"
fetch "${base}/checksums.txt" "$checksums"

expected=$(awk -v name="$asset" '$2 == name { print $1 }' "$checksums")
[ -n "$expected" ] || { echo "checksum not found for $asset" >&2; exit 1; }

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$tmp" | awk '{print $1}')
fi
[ "$actual" = "$expected" ] || { echo "checksum mismatch" >&2; exit 1; }

chmod +x "$tmp"
mv "$tmp" "$DEST"
trap - EXIT INT TERM
rm -f "$checksums"
echo "installed $asset -> $DEST"
"$DEST" -version
