#!/usr/bin/env sh
# Install lazyaz from GitHub Releases.
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/karlssonsimon/lazyaz/master/install.sh | sh
#   curl -sSfL https://raw.githubusercontent.com/karlssonsimon/lazyaz/master/install.sh | sh -s -- --version v0.1.0
#
# Env overrides:
#   LAZYAZ_VERSION   release tag to install (default: latest)
#   LAZYAZ_INSTALL_DIR  target directory (default: $HOME/.local/bin, or /usr/local/bin if writable)

set -eu

REPO="karlssonsimon/lazyaz"
BIN="lazyaz"
VERSION="${LAZYAZ_VERSION:-latest}"
INSTALL_DIR="${LAZYAZ_INSTALL_DIR:-}"

log() { printf '==> %s\n' "$*"; }
err() { printf 'error: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --dir) INSTALL_DIR="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) err "unknown argument: $1" ;;
  esac
done

need() { command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"; }
need uname
need tar
if command -v curl >/dev/null 2>&1; then
  FETCH="curl -sSfL"
elif command -v wget >/dev/null 2>&1; then
  FETCH="wget -qO-"
else
  err "need curl or wget"
fi

OS="$(uname -s)"
case "$OS" in
  Linux)  OS=linux ;;
  Darwin) OS=darwin ;;
  *) err "unsupported OS: $OS" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) err "unsupported architecture: $ARCH" ;;
esac

if [ "$VERSION" = "latest" ]; then
  log "resolving latest release"
  VERSION="$($FETCH "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
  [ -n "$VERSION" ] || err "could not determine latest version"
fi

NUM="${VERSION#v}"
ARCHIVE="${BIN}_${NUM}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

log "downloading $ARCHIVE ($VERSION)"
$FETCH "$BASE_URL/$ARCHIVE" > "$TMP/$ARCHIVE" || err "download failed"
$FETCH "$BASE_URL/checksums.txt" > "$TMP/checksums.txt" || err "checksum download failed"

log "verifying checksum"
EXPECTED="$(grep " $ARCHIVE\$" "$TMP/checksums.txt" | awk '{print $1}')"
[ -n "$EXPECTED" ] || err "no checksum entry for $ARCHIVE"

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "$TMP/$ARCHIVE" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL="$(shasum -a 256 "$TMP/$ARCHIVE" | awk '{print $1}')"
else
  err "need sha256sum or shasum"
fi
[ "$ACTUAL" = "$EXPECTED" ] || err "checksum mismatch ($ACTUAL != $EXPECTED)"

log "extracting"
tar -xzf "$TMP/$ARCHIVE" -C "$TMP"
[ -f "$TMP/$BIN" ] || err "binary not found in archive"

if [ -z "$INSTALL_DIR" ]; then
  if [ -w /usr/local/bin ]; then
    INSTALL_DIR=/usr/local/bin
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi
mkdir -p "$INSTALL_DIR"

install -m 0755 "$TMP/$BIN" "$INSTALL_DIR/$BIN"
log "installed $BIN $VERSION to $INSTALL_DIR/$BIN"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) log "note: $INSTALL_DIR is not on PATH — add it to your shell profile" ;;
esac
