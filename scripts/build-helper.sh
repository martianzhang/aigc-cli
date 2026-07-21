#!/bin/bash
# Compile the C helper library for the current platform.
# Usage: bash scripts/build-helper.sh
set -euo pipefail

# Detect platform
RAW_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
RPATH="\$ORIGIN"

# Normalize OS name for Windows (MSYS/MINGW/CYGWIN)
case "$RAW_OS" in
  mingw*|msys*|cygwin*) OS="windows" ;;
  darwin*) OS="darwin" ;;
  linux*)  OS="linux" ;;
  *) echo "unsupported: $RAW_OS"; exit 1 ;;
esac

case "$OS" in
  darwin) LIB_EXT="dylib"; SONAME="-install_name @rpath/libaigc-sherpa-helper.dylib"; RPATH="@loader_path" ;;
  linux)  LIB_EXT="so";   SONAME="-Wl,-soname,libaigc-sherpa-helper.so" ;;
  windows) LIB_EXT="dll"; SONAME="" ;;
esac

# Find sherpa-onnx headers and libs in Go module cache
GOMODCACHE=$(go env GOMODCACHE)
PKG_NAME="sherpa-onnx-go-$OS"
case "$OS-$ARCH" in
  darwin-arm64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/$PKG_NAME@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/aarch64-apple-darwin" ;;
  darwin-x86_64|darwin-amd64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/$PKG_NAME@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/x86_64-apple-darwin" ;;
  linux-aarch64|linux-arm64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/$PKG_NAME@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/aarch64-unknown-linux-gnu" ;;
  linux-x86_64|linux-amd64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/$PKG_NAME@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/x86_64-unknown-linux-gnu" ;;
  windows-x86_64|windows-amd64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/$PKG_NAME@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/x86_64-pc-windows-gnu" ;;
  *) echo "unsupported arch: $OS-$ARCH"; exit 1 ;;
esac

OUTPUT="libaigc-sherpa-helper.$LIB_EXT"
SRC="scripts/helper.c"

echo "Building $OUTPUT for $OS-$ARCH..."
echo "  sherpa headers: $SHERPA_DIR"
echo "  lib dir: $LIB_DIR"

gcc -shared $SONAME -o "$OUTPUT" \
  -I"$SHERPA_DIR" \
  -L"$LIB_DIR" \
  -lsherpa-onnx-c-api \
  -Wl,-rpath,"$RPATH" \
  "$SRC"

echo "  done: $(ls -lh "$OUTPUT" | awk '{print $5}')"
