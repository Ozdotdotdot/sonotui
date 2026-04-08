#!/usr/bin/env sh
# Install sonotui (terminal UI) from the latest GitHub Release.
# Usage: curl -fsSL https://raw.githubusercontent.com/ozdotdotdot/sonotui/main/scripts/install-tui.sh | sh
set -e

REPO="ozdotdotdot/sonotui"
INSTALL_DIR="${SONOTUI_INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS and architecture, map to Rust target triple
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
  linux)
    case "$ARCH" in
      x86_64)        TARGET="x86_64-unknown-linux-gnu" ;;
      aarch64|arm64) TARGET="aarch64-unknown-linux-gnu" ;;
      *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
    esac
    ;;
  darwin)
    case "$ARCH" in
      x86_64)        TARGET="x86_64-apple-darwin" ;;
      aarch64|arm64) TARGET="aarch64-apple-darwin" ;;
      *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
    esac
    ;;
  *)
    echo "Unsupported OS: $OS" >&2
    exit 1
    ;;
esac

# Resolve latest release version
VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
  | grep '"tag_name"' | head -1 | cut -d'"' -f4)

if [ -z "$VERSION" ]; then
  echo "Could not determine latest release version." >&2
  exit 1
fi

FILENAME="sonotui-${TARGET}-${VERSION}.tar.gz"
URL="https://github.com/$REPO/releases/download/$VERSION/$FILENAME"

echo "Installing sonotui $VERSION for $TARGET..."
mkdir -p "$INSTALL_DIR"
curl -fsSL "$URL" | tar -xz -C "$INSTALL_DIR"
mv "$INSTALL_DIR/sonotui-${TARGET}" "$INSTALL_DIR/sonotui"
chmod +x "$INSTALL_DIR/sonotui"

echo ""
echo "sonotui installed to $INSTALL_DIR/sonotui"
echo ""
echo "Run 'sonotui' — it will find your daemon automatically."
