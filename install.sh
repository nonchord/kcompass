#!/bin/sh
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nonchord/kcompass/main/install.sh | sh
#   curl -fsSL ... | sh -s -- --dir /usr/local/bin
#   curl -fsSL ... | sh -s -- --version 0.2.0
set -eu

REPO="nonchord/kcompass"
INSTALL_DIR="${HOME}/.local/bin"
VERSION=""

while [ $# -gt 0 ]; do
  case "$1" in
    --dir)     INSTALL_DIR="$2"; shift 2 ;;
    --version) VERSION="$2";     shift 2 ;;
    -h|--help)
      echo "Usage: install.sh [--dir DIR] [--version VERSION]"
      echo "  --dir DIR        Install directory (default: ~/.local/bin)"
      echo "  --version VER    Version to install (default: latest)"
      exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

if [ -z "$VERSION" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | cut -d'"' -f4)"
  if [ -z "$VERSION" ]; then
    echo "Failed to determine latest version." >&2
    exit 1
  fi
fi

# Strip leading v for the archive name (goreleaser uses bare version).
VERSION_BARE="${VERSION#v}"

ARCHIVE="kcompass_${VERSION_BARE}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading kcompass ${VERSION} for ${OS}/${ARCH}..."
curl -fsSL -o "${TMPDIR}/${ARCHIVE}" "$URL"
curl -fsSL -o "${TMPDIR}/checksums.txt" "$CHECKSUM_URL"

# Verify checksum.
(cd "$TMPDIR" && grep "$ARCHIVE" checksums.txt | sha256sum -c --quiet -) || {
  echo "Checksum verification failed!" >&2
  exit 1
}

tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

mkdir -p "$INSTALL_DIR"
mv "${TMPDIR}/kcompass" "${INSTALL_DIR}/kcompass"
chmod +x "${INSTALL_DIR}/kcompass"

echo "Installed kcompass to ${INSTALL_DIR}/kcompass"

# Check if INSTALL_DIR is in PATH.
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) echo "Note: ${INSTALL_DIR} is not in your PATH. Add it with:"
     echo "  export PATH=\"${INSTALL_DIR}:\$PATH\"" ;;
esac
