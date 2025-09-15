#!/bin/bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN_LOCAL="$PROJECT_DIR/bin/bookmark"
LINK_NAME="/usr/local/bin/bm"

echo "Building project..."
cd "$PROJECT_DIR"
./scripts/build.sh

echo "Linking $BIN_LOCAL -> $LINK_NAME"
if [ ! -f "$BIN_LOCAL" ]; then
  echo "Error: built binary not found at $BIN_LOCAL" >&2
  exit 1
fi

if [ ! -w "$(dirname "$LINK_NAME")" ]; then
  echo "Creating symlink with sudo (may prompt for password)..."
  sudo ln -sf "$BIN_LOCAL" "$LINK_NAME"
else
  ln -sf "$BIN_LOCAL" "$LINK_NAME"
fi

echo "Done. Test with: bm --help"

