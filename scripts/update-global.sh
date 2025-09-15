#!/bin/bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN_LOCAL="$PROJECT_DIR/bin/bookmark"
LINK_NAME="/usr/local/bin/bm"

echo "Pulling latest changes..."
cd "$PROJECT_DIR"
git pull --rebase

echo "Rebuilding..."
./scripts/build.sh

if [ -L "$LINK_NAME" ]; then
  TARGET="$(readlink "$LINK_NAME" || true)"
  if [ "$TARGET" != "$BIN_LOCAL" ]; then
    echo "Updating symlink to point to $BIN_LOCAL"
    if [ ! -w "$(dirname "$LINK_NAME")" ]; then
      sudo ln -sf "$BIN_LOCAL" "$LINK_NAME"
    else
      ln -sf "$BIN_LOCAL" "$LINK_NAME"
    fi
  fi
else
  echo "Creating global symlink at $LINK_NAME"
  if [ ! -w "$(dirname "$LINK_NAME")" ]; then
    sudo ln -sf "$BIN_LOCAL" "$LINK_NAME"
  else
    ln -sf "$BIN_LOCAL" "$LINK_NAME"
  fi
fi

echo "Updated. Version:"
bm --help | head -n 1 || true

