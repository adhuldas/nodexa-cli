#!/usr/bin/env bash
# install.sh -- One-line installer for nodex CLI
# Installs directly to /usr/local/bin so it is immediately available on PATH.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/adhuldas/nodexa-cli/main/install.sh | sh

set -e

REPO="adhuldas/nodexa-cli"
INSTALL_DIR="/usr/local/bin"
BIN_NAME="nodex"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH" && exit 1 ;;
esac

echo "==> Installing nodex CLI for ${OS}-${ARCH}..."

# If Go is installed and repository is cloned locally, build directly
if [ -f "./cmd/nodex/main.go" ] && command -v go >/dev/null 2>&1; then
    echo "  Building from local source..."
    go build -ldflags "-s -w" -o "${BIN_NAME}" ./cmd/nodex
    TARGET="${INSTALL_DIR}/${BIN_NAME}"
    if [ -w "${INSTALL_DIR}" ]; then
        mv "${BIN_NAME}" "${TARGET}"
    else
        echo "  Requesting sudo permissions to install to ${INSTALL_DIR}..."
        sudo mv "${BIN_NAME}" "${TARGET}"
    fi
    chmod +x "${TARGET}"
    echo "==> Successfully installed ${BIN_NAME} to ${TARGET}"
    "${TARGET}" --version
    exit 0
fi

# Otherwise, fetch latest release asset from GitHub
LATEST_TAG=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$LATEST_TAG" ]; then
    # Fallback to building with go if available
    if command -v go >/dev/null 2>&1; then
        echo "  No GitHub release found yet. Building with go install..."
        go install "github.com/${REPO}/cmd/nodex@latest"
        GOPATH_BIN="$(go env GOPATH)/bin/nodex"
        if [ -f "$GOPATH_BIN" ]; then
            if [ -w "${INSTALL_DIR}" ]; then
                cp "$GOPATH_BIN" "${INSTALL_DIR}/${BIN_NAME}"
            else
                sudo cp "$GOPATH_BIN" "${INSTALL_DIR}/${BIN_NAME}"
            fi
            echo "==> Successfully installed ${BIN_NAME} to ${INSTALL_DIR}/${BIN_NAME}"
            "${INSTALL_DIR}/${BIN_NAME}" --version
            exit 0
        fi
    fi
    echo "Error: Could not find release or local go toolchain."
    exit 1
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/nodex_${OS}_${ARCH}.tar.gz"
echo "  Downloading ${DOWNLOAD_URL}..."
TMP_DIR=$(mktemp -d)
curl -fsSL "$DOWNLOAD_URL" | tar -xz -C "$TMP_DIR"

TARGET="${INSTALL_DIR}/${BIN_NAME}"
if [ -w "${INSTALL_DIR}" ]; then
    mv "${TMP_DIR}/${BIN_NAME}" "${TARGET}"
else
    sudo mv "${TMP_DIR}/${BIN_NAME}" "${TARGET}"
fi
rm -rf "$TMP_DIR"
chmod +x "${TARGET}"

echo "==> Successfully installed ${BIN_NAME} to ${TARGET}"
"${TARGET}" --version
