#!/bin/bash
# Compile the C helper library for the current platform.
# Usage: bash scripts/build-helper.sh
#
# Honors GOOS/GOARCH so release CI can cross-compile the helper to match the
# binary it ships next to (e.g. darwin/amd64 on an arm64 macOS runner).
set -euo pipefail

# Target platform: GOOS/GOARCH first (go build uses the same), host otherwise.
if command -v go >/dev/null 2>&1; then
  RAW_OS=$(go env GOOS)
  ARCH=$(go env GOARCH)
else
  RAW_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)
fi
RPATH="\$ORIGIN"

# Normalize OS name
case "$RAW_OS" in
  mingw*|msys*|cygwin*|windows) OS="windows" ;;
  darwin*) OS="darwin" ;;
  linux*)  OS="linux" ;;
  *) echo "unsupported: $RAW_OS"; exit 1 ;;
esac

# macOS ships a universal SDK, so clang can emit the other architecture.
ARCH_FLAG=""
if [ "$OS" = "darwin" ]; then
  case "$ARCH" in
    x86_64|amd64)  ARCH_FLAG="-arch x86_64" ;;
    aarch64|arm64) ARCH_FLAG="-arch arm64" ;;
    *) echo "unsupported arch: $ARCH"; exit 1 ;;
  esac
fi

# Go module uses "macos" not "darwin"
case "$OS" in
  darwin) GO_OS="macos" ;;
  *)     GO_OS="$OS" ;;
esac

case "$OS" in
  darwin) LIB_EXT="dylib"; SONAME="-install_name @rpath/libaigc-sherpa-helper.dylib"; RPATH="@loader_path" ;;
  linux)  LIB_EXT="so";   SONAME="-Wl,-soname,libaigc-sherpa-helper.so" ;;
  windows) LIB_EXT="dll"; SONAME="" ;;
esac

# Find sherpa-onnx headers and libs in Go module cache
GOMODCACHE=$(go env GOMODCACHE)
PKG_NAME="sherpa-onnx-go-$GO_OS"
case "$OS-$ARCH" in
  darwin-arm64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/$PKG_NAME@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/aarch64-apple-darwin" ;;
  darwin-x86_64|darwin-amd64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/$PKG_NAME@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/x86_64-apple-darwin" ;;
  linux-aarch64|linux-arm64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/$PKG_NAME@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/aarch64-unknown-linux-gnu" ;;
  linux-x86_64|linux-amd64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/$PKG_NAME@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/x86_64-unknown-linux-gnu" ;;
  windows-x86_64|windows-amd64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/$PKG_NAME@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/x86_64-pc-windows-gnu" ;;
  *) echo "unsupported arch: $OS-$ARCH"; exit 1 ;;
esac

OUTPUT="libaigc-sherpa-helper.$LIB_EXT"
[ "$OS" = "windows" ] && OUTPUT="aigc-sherpa-helper.dll"
SRC="scripts/helper.c"

echo "Building $OUTPUT for $OS-$ARCH..."
echo "  sherpa headers: $SHERPA_DIR"
echo "  lib dir: $LIB_DIR"

gcc -shared $ARCH_FLAG $SONAME -o "$OUTPUT" \
  -I"$SHERPA_DIR" \
  -L"$LIB_DIR" \
  -lsherpa-onnx-c-api \
  -Wl,-rpath,"$RPATH" \
  "$SRC"

echo "  done: $(ls -lh "$OUTPUT" | awk '{print $5}')"
