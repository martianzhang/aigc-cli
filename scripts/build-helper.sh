#!/bin/bash
# Compile the C helper library for the current platform.
# Usage: bash scripts/build-helper.sh
set -euo pipefail

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
RPATH="\$ORIGIN"
case "$OS" in
  darwin) LIB_EXT="dylib"; SONAME="-install_name @rpath/libaigc-sherpa-helper.dylib"; RPATH="@loader_path" ;;
  linux)  LIB_EXT="so";   SONAME="-Wl,-soname,libaigc-sherpa-helper.so" ;;
  *) echo "unsupported: $OS"; exit 1 ;;
esac

# Find sherpa-onnx headers and libs in Go module cache
GOMODCACHE=$(go env GOMODCACHE)
case "$OS-$ARCH" in
  darwin-arm64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/sherpa-onnx-go-macos@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/aarch64-apple-darwin" ;;
  darwin-x86_64|darwin-amd64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/sherpa-onnx-go-macos@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/x86_64-apple-darwin" ;;
  linux-aarch64|linux-arm64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/sherpa-onnx-go-linux@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/aarch64-unknown-linux-gnu" ;;
  linux-x86_64|linux-amd64) SHERPA_DIR="$GOMODCACHE/github.com/k2-fsa/sherpa-onnx-go-linux@v1.13.4"; LIB_DIR="$SHERPA_DIR/lib/x86_64-unknown-linux-gnu" ;;
  *) echo "unsupported arch: $OS-$ARCH"; exit 1 ;;
esac

OUTPUT="libaigc-sherpa-helper.$LIB_EXT"
SRC="internal/audio/helper.c"

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
