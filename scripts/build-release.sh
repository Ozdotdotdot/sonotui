#!/usr/bin/env bash
# Build release binaries for all supported platforms.
# Usage: ./scripts/build-release.sh [version]
# Output: dist/sonotuid-{os}-{arch}-{version}.tar.gz
#         dist/sonotui-{target}-{version}.tar.gz
#
# Note: Rust cross-compilation for non-native targets requires the relevant
# rustup targets and, for Linux arm64 on a Linux x86_64 host, a cross-linker
# (e.g. gcc-aarch64-linux-gnu). The GitHub Actions release workflow handles
# this automatically using native runners per target.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-$(git -C "$REPO_ROOT" describe --tags --always 2>/dev/null || echo dev)}"
DIST="$REPO_ROOT/dist"

mkdir -p "$DIST"
echo "Building sonotui $VERSION → $DIST"

# ── Daemon (Go) ──────────────────────────────────────────────────────────────

DAEMON_TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
)

for TARGET in "${DAEMON_TARGETS[@]}"; do
  OS="${TARGET%/*}"
  ARCH="${TARGET#*/}"
  OUT="sonotuid-${OS}-${ARCH}"
  echo "  daemon $OS/$ARCH"
  GOOS="$OS" GOARCH="$ARCH" go build \
    -o "$DIST/$OUT" \
    "$REPO_ROOT/cmd/sonotuid/"
  tar -czf "$DIST/${OUT}-${VERSION}.tar.gz" -C "$DIST" "$OUT"
  rm "$DIST/$OUT"
done

# ── TUI (Rust) ───────────────────────────────────────────────────────────────

RUST_TARGETS=(
  "x86_64-unknown-linux-gnu"
  "aarch64-unknown-linux-gnu"
  "x86_64-apple-darwin"
  "aarch64-apple-darwin"
)

for TARGET in "${RUST_TARGETS[@]}"; do
  echo "  tui $TARGET"
  cargo build --release \
    --target "$TARGET" \
    --manifest-path "$REPO_ROOT/tui-rs/Cargo.toml" \
    --quiet
  cp "$REPO_ROOT/tui-rs/target/$TARGET/release/sonotui" \
     "$DIST/sonotui-${TARGET}"
  tar -czf "$DIST/sonotui-${TARGET}-${VERSION}.tar.gz" \
    -C "$DIST" "sonotui-${TARGET}"
  rm "$DIST/sonotui-${TARGET}"
done

echo ""
echo "Done. Artifacts in $DIST:"
ls -lh "$DIST"/*.tar.gz
