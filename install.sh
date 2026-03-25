#!/bin/sh
set -e

REPO="BenjaminBenetti/fleet-man"
INSTALL_DIR="/usr/local/bin"
BINARY="fleet"

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo "Error: unsupported architecture: $ARCH"
        exit 1
        ;;
esac

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$OS" != "linux" ]; then
    echo "Error: unsupported OS: $OS (only linux is supported)"
    exit 1
fi

ASSET="fleet-${OS}-${ARCH}"

# Get latest release tag
echo "Fetching latest release..."
TAG=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | cut -d '"' -f 4)

if [ -z "$TAG" ]; then
    echo "Error: could not determine latest release"
    exit 1
fi

URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"

echo "Downloading fleet ${TAG} (${OS}/${ARCH})..."
TMP=$(mktemp)
HTTP_CODE=$(curl -sL -o "$TMP" -w "%{http_code}" "$URL")

if [ "$HTTP_CODE" != "200" ]; then
    rm -f "$TMP"
    echo "Error: download failed (HTTP ${HTTP_CODE})"
    echo "URL: ${URL}"
    exit 1
fi

chmod +x "$TMP"

# Install — use sudo if needed
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP" "${INSTALL_DIR}/${BINARY}"
else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
fi

echo "fleet ${TAG} installed to ${INSTALL_DIR}/${BINARY}"
