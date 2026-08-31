#!/bin/sh
set -e

# finst - One-line installer for macOS and Linux
# Repository: https://github.com/arisolta/finst

REPO="arisolta/finst"
INSTALL_DIR="${BIN_DIR:-$HOME/.local/bin}"

# Text formatting
BOLD="\033[1m"
GREEN="\033[32m"
BLUE="\033[34m"
YELLOW="\033[33m"
RED="\033[31m"
RESET="\033[0m"

printf "${BOLD}${BLUE}==>${RESET} ${BOLD}Installing finst (Financial Terminal CLI)...${RESET}\n"

# 1. Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    darwin) OS="darwin" ;;
    linux) OS="linux" ;;
    *)
        printf "${RED}Error: Unsupported operating system: %s${RESET}\n" "$OS"
        printf "For Windows, run the PowerShell installer: irm https://raw.githubusercontent.com/%s/main/install.ps1 | iex\n" "$REPO"
        exit 1
        ;;
esac

# 2. Detect Architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        printf "${RED}Error: Unsupported architecture: %s${RESET}\n" "$ARCH"
        exit 1
        ;;
esac

# 3. Find latest release tag
printf "${BLUE}==>${RESET} Fetching latest release info from GitHub...\n"
TAG=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)

if [ -z "$TAG" ]; then
    TAG="v1.0.2"
fi

FILENAME="finst_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${FILENAME}"

# 4. Download and extract
TMP_DIR="$(mktemp -d)"
cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

printf "${BLUE}==>${RESET} Downloading ${BOLD}%s${RESET} (%s/%s)...\n" "$TAG" "$OS" "$ARCH"
if ! curl -sSL -f -o "$TMP_DIR/$FILENAME" "$DOWNLOAD_URL"; then
    printf "${YELLOW}Release asset not yet available on GitHub Releases for %s. Attempting fallback build...${RESET}\n" "$TAG"
    if command -v go >/dev/null 2>&1; then
        printf "${BLUE}==>${RESET} Go compiler detected. Building directly from source...\n"
        go install "github.com/${REPO}/cmd/finst@latest"
        printf "${GREEN}✓ Successfully installed finst via go install!${RESET}\n"
        exit 0
    else
        printf "${RED}Error: Failed to download %s from %s${RESET}\n" "$FILENAME" "$DOWNLOAD_URL"
        exit 1
    fi
fi

mkdir -p "$INSTALL_DIR"
tar -xzf "$TMP_DIR/$FILENAME" -C "$TMP_DIR"

# Move binary to target directory
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/finst" "$INSTALL_DIR/finst"
    chmod +x "$INSTALL_DIR/finst"
else
    printf "${YELLOW}Requires root privileges to install to %s${RESET}\n" "$INSTALL_DIR"
    sudo mv "$TMP_DIR/finst" "$INSTALL_DIR/finst"
    sudo chmod +x "$INSTALL_DIR/finst"
fi

printf "${GREEN}✓ finst %s installed successfully to %s/finst!${RESET}\n\n" "$TAG" "$INSTALL_DIR"

# 5. Check if INSTALL_DIR is in PATH
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
        printf "${YELLOW}${BOLD}Notice:${RESET} %s is not in your \$PATH.\n" "$INSTALL_DIR"
        
        SHELL_NAME="$(basename "$SHELL")"
        RC_FILE=""
        if [ "$SHELL_NAME" = "zsh" ]; then
            RC_FILE="$HOME/.zshrc"
        elif [ "$SHELL_NAME" = "bash" ]; then
            RC_FILE="$HOME/.bashrc"
        fi

        if [ -n "$RC_FILE" ]; then
            printf "To add it automatically, run:\n"
            printf "  ${BOLD}echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> %s && source %s${RESET}\n\n" "$RC_FILE" "$RC_FILE"
        fi
        ;;
esac

printf "${BOLD}Quick Start:${RESET}\n"
printf "  finst AAPL          # Analyze Apple\n"
printf "  finst MC.PA         # Analyze LVMH (Euronext Paris)\n"
printf "  finst NVDA --export csv\n\n"
