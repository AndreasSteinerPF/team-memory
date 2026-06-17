#!/bin/sh
# install.sh — fetch a prebuilt `tm` binary from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/AndreasSteinerPF/team-memory/main/install.sh | sh
#
# Env vars:
#   TM_VERSION       version to install (default: latest, e.g. "v0.5.0")
#   TM_INSTALL_DIR   where to drop the binary (default: $HOME/.local/bin)
#
# Verifies the archive's SHA-256 against the release's checksums.txt before
# installing. POSIX sh, no bashisms.

set -eu

REPO="AndreasSteinerPF/team-memory"
INSTALL_DIR="${TM_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${TM_VERSION:-latest}"

err() { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }

need() { command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"; }
need curl
need tar
need uname

# --- detect platform ---------------------------------------------------------
os_raw=$(uname -s)
case "$os_raw" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *)      err "unsupported OS: $os_raw (Windows: use install.ps1 or Scoop)" ;;
esac

arch_raw=$(uname -m)
case "$arch_raw" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *)             err "unsupported architecture: $arch_raw" ;;
esac

# --- resolve version ---------------------------------------------------------
if [ "$VERSION" = "latest" ]; then
  info "Resolving latest release..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
  [ -n "$VERSION" ] || err "could not resolve latest release tag from GitHub API"
fi

# GoReleaser archive names are tm_<version-without-v>_<os>_<arch>.tar.gz
ver_no_v=${VERSION#v}
archive="tm_${ver_no_v}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$VERSION/$archive"
sums_url="https://github.com/$REPO/releases/download/$VERSION/checksums.txt"

info "Downloading $archive ($VERSION)..."
tmpdir=$(mktemp -d 2>/dev/null || mktemp -d -t tm-install)
trap 'rm -rf "$tmpdir"' EXIT INT TERM

curl -fsSL -o "$tmpdir/$archive" "$url" \
  || err "download failed: $url"
curl -fsSL -o "$tmpdir/checksums.txt" "$sums_url" \
  || err "checksums download failed: $sums_url"

# --- verify checksum ---------------------------------------------------------
if command -v shasum >/dev/null 2>&1; then
  sumcmd="shasum -a 256"
elif command -v sha256sum >/dev/null 2>&1; then
  sumcmd="sha256sum"
else
  err "missing sha256 tool (need shasum or sha256sum)"
fi

actual=$(cd "$tmpdir" && $sumcmd "$archive" | awk '{print $1}')
expected=$(grep -E "  ${archive}$" "$tmpdir/checksums.txt" | awk '{print $1}')
[ -n "$expected" ] || err "no checksum entry for $archive in checksums.txt"
[ "$actual" = "$expected" ] || err "checksum mismatch for $archive (expected $expected, got $actual)"
info "Checksum OK."

# --- extract and install -----------------------------------------------------
tar -xzf "$tmpdir/$archive" -C "$tmpdir"
[ -f "$tmpdir/tm" ] || err "archive did not contain a tm binary"

mkdir -p "$INSTALL_DIR"
mv "$tmpdir/tm" "$INSTALL_DIR/tm"
chmod +x "$INSTALL_DIR/tm"
info "Installed tm $VERSION to $INSTALL_DIR/tm"

# --- PATH hint ---------------------------------------------------------------
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) printf '\nNote: %s is not on your PATH. Add it (e.g. in ~/.bashrc or ~/.zshrc):\n  export PATH="%s:$PATH"\n' "$INSTALL_DIR" "$INSTALL_DIR" ;;
esac

"$INSTALL_DIR/tm" version 2>/dev/null || true
