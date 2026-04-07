#!/usr/bin/env sh
# Install sonotuid (daemon) from the latest GitHub Release.
# Usage: curl -fsSL https://raw.githubusercontent.com/ozdotdotdot/sonotui/main/scripts/install.sh | sh
set -e

REPO="ozdotdotdot/sonotui"
INSTALL_DIR="${SONOTUI_INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)         ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

case "$OS" in
  linux|darwin) ;;
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

FILENAME="sonotuid-${OS}-${ARCH}-${VERSION}.tar.gz"
URL="https://github.com/$REPO/releases/download/$VERSION/$FILENAME"

echo "Installing sonotuid $VERSION for $OS/$ARCH..."
mkdir -p "$INSTALL_DIR"
curl -fsSL "$URL" | tar -xz -C "$INSTALL_DIR"
chmod +x "$INSTALL_DIR/sonotuid"

echo ""
echo "sonotuid installed to $INSTALL_DIR/sonotuid"
echo ""
echo "Next steps:"
echo "  1. Create ~/.config/sonotuid/config.toml  (see docs/config.md)"
echo "  2. Run 'sonotuid --install' to register the systemd user service"
echo "  3. Run 'systemctl --user start sonotuid' to start it"
