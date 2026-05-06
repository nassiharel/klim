#!/usr/bin/env bash

# klim installer — downloads the latest release binary for Linux/macOS.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nassiharel/klim/main/install.sh | bash
#   curl -fsSL ... | bash -s -- --version v1.2.3
#   curl -fsSL ... | bash -s -- --no-sudo --install-dir ~/.local/bin
#   wget -qO- https://raw.githubusercontent.com/nassiharel/klim/main/install.sh | bash

set -euo pipefail

: "${BINARY_NAME:="klim"}"
: "${USE_SUDO:="true"}"
: "${VERIFY_CHECKSUM:="true"}"
: "${INSTALL_DIR:="/usr/local/bin"}"

GITHUB_REPO="nassiharel/klim"

HAS_CURL="$(command -v curl >/dev/null 2>&1 && echo true || echo false)"
HAS_WGET="$(command -v wget >/dev/null 2>&1 && echo true || echo false)"
HAS_TAR="$(command -v tar >/dev/null 2>&1 && echo true || echo false)"

# Colors (if terminal supports them)
if [ -t 1 ]; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  CYAN='\033[0;36m'
  NC='\033[0m'
else
  RED='' GREEN='' YELLOW='' CYAN='' NC=''
fi

info()  { echo -e "${GREEN}[info]${NC}  $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC}  $*"; }
error() { echo -e "${RED}[error]${NC} $*" >&2; }

# initArch maps uname -m to Go architecture names.
initArch() {
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64|amd64)   ARCH="amd64" ;;
    aarch64|arm64)   ARCH="arm64" ;;
    armv7*)          ARCH="armv7" ;;
    armv6*)          ARCH="armv6" ;;
    *)
      warn "Unknown architecture: $ARCH — will attempt go install fallback."
      ARCH="$ARCH"
      ;;
  esac
}

# initOS maps uname to Go OS names.
initOS() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$OS" in
    darwin)  OS="darwin" ;;
    linux)   OS="linux" ;;
    freebsd) OS="freebsd" ;;
    mingw*|cygwin*|msys*)
      error "Windows detected. Please use the PowerShell installer instead:"
      error "  irm https://raw.githubusercontent.com/nassiharel/klim/main/install.ps1 | iex"
      exit 1
      ;;
    *)
      warn "Unknown OS: $OS — will attempt go install fallback."
      ;;
  esac
}

# verifySupported checks that the os/arch combination is supported
# and that required tools are available.
verifySupported() {
  local supported="darwin-amd64 darwin-arm64 linux-amd64 linux-arm64"
  if echo "$supported" | grep -qw "${OS}-${ARCH}"; then
    HAS_PREBUILT="true"
  else
    HAS_PREBUILT="false"
    warn "No prebuilt binary for ${OS}/${ARCH}."
  fi

  if [ "$HAS_PREBUILT" = "true" ]; then
    if [ "$HAS_CURL" != "true" ] && [ "$HAS_WGET" != "true" ]; then
      error "Either curl or wget is required to download klim."
      exit 1
    fi

    if [ "$HAS_TAR" != "true" ]; then
      error "tar is required to extract the archive."
      exit 1
    fi
  fi
}

# goInstallFallback builds from source using go install.
goInstallFallback() {
  if ! command -v go >/dev/null 2>&1; then
    error "No prebuilt binary for ${OS}/${ARCH} and Go is not installed."
    error "Install Go (https://go.dev/dl/) or use a supported platform."
    exit 1
  fi

  info "Building from source via ${CYAN}go install${NC}..."
  go install "github.com/nassiharel/klim/cmd/klim@${TAG}"

  local gobin
  gobin="$(go env GOPATH)/bin"
  if [ ! -f "${gobin}/${BINARY_NAME}" ]; then
    gobin="$(go env GOBIN)"
  fi

  if [ ! -f "${gobin}/${BINARY_NAME}" ]; then
    error "go install succeeded but binary not found in GOPATH/bin or GOBIN."
    exit 1
  fi

  if [ ! -d "$INSTALL_DIR" ]; then
    runAsRoot mkdir -p "$INSTALL_DIR"
  fi

  runAsRoot cp "${gobin}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  runAsRoot chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
  info "Installed ${GREEN}${BINARY_NAME}${NC} (built from source) to ${INSTALL_DIR}/${BINARY_NAME}"
}

# runAsRoot runs a command with sudo if needed.
runAsRoot() {
  if [ "$(id -u)" -ne 0 ] && [ "$USE_SUDO" = "true" ]; then
    sudo "$@"
  else
    "$@"
  fi
}

# getLatestVersion fetches the latest release tag from GitHub.
getLatestVersion() {
  if [ -n "${DESIRED_VERSION:-}" ]; then
    TAG="$DESIRED_VERSION"
    # Ensure tag starts with 'v'
    if [[ "$TAG" != v* ]]; then
      TAG="v$TAG"
    fi
    return
  fi

  local url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
  local response=""

  if [ "$HAS_CURL" = "true" ]; then
    response=$(curl -sSL "$url" 2>/dev/null) || true
  elif [ "$HAS_WGET" = "true" ]; then
    response=$(wget -qO- "$url" 2>/dev/null) || true
  fi

  TAG=$(echo "$response" | grep '"tag_name"' | sed -E 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/')

  if [ -z "$TAG" ]; then
    # Fallback: follow the /releases/latest redirect.
    if [ "$HAS_CURL" = "true" ]; then
      TAG=$(curl -sSLI -o /dev/null -w '%{url_effective}' "https://github.com/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+[^ ]*$') || true
    fi
  fi

  if [ -z "$TAG" ]; then
    error "Could not determine the latest version."
    error "This may be caused by GitHub API rate limiting."
    error "Please specify a version with --version, e.g. --version v1.0.0"
    error "Check releases at: https://github.com/${GITHUB_REPO}/releases"
    exit 1
  fi
}

# checkInstalledVersion checks if the desired version is already installed.
checkInstalledVersion() {
  if [ -f "${INSTALL_DIR}/${BINARY_NAME}" ]; then
    local installed
    installed=$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1) || true
    local desired
    desired="${TAG#v}"
    if [ "$installed" = "$desired" ]; then
      info "${BINARY_NAME} ${TAG} is already installed."
      return 0
    fi
  fi
  return 1
}

# downloadFile downloads the release archive and checksums to a temp directory.
downloadFile() {
  local version="${TAG#v}"
  DIST_FILE="${BINARY_NAME}_${version}_${OS}_${ARCH}.tar.gz"
  DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${TAG}/${DIST_FILE}"
  CHECKSUM_URL="https://github.com/${GITHUB_REPO}/releases/download/${TAG}/checksums.txt"

  TMP_DIR=$(mktemp -d -t klim-install-XXXXXX)
  TMP_FILE="${TMP_DIR}/${DIST_FILE}"
  TMP_CHECKSUM="${TMP_DIR}/checksums.txt"

  info "Downloading ${CYAN}${BINARY_NAME} ${TAG}${NC} for ${OS}/${ARCH}..."

  if [ "$HAS_CURL" = "true" ]; then
    curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"
    curl -fsSL "$CHECKSUM_URL" -o "$TMP_CHECKSUM"
  elif [ "$HAS_WGET" = "true" ]; then
    wget -qO "$TMP_FILE" "$DOWNLOAD_URL"
    wget -qO "$TMP_CHECKSUM" "$CHECKSUM_URL"
  fi
}

# verifyChecksum validates the SHA256 checksum of the downloaded archive.
verifyChecksum() {
  if [ "$VERIFY_CHECKSUM" != "true" ]; then
    return
  fi

  info "Verifying checksum..."

  local expected
  expected=$(grep -F "${DIST_FILE}" "$TMP_CHECKSUM" | awk '{print $1}')

  if [ -z "$expected" ]; then
    warn "Checksum entry not found for ${DIST_FILE}. Skipping verification."
    return
  fi

  local actual=""
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$TMP_FILE" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$TMP_FILE" | awk '{print $1}')
  elif command -v openssl >/dev/null 2>&1; then
    actual=$(openssl sha256 "$TMP_FILE" | awk '{print $NF}')
  else
    warn "No SHA256 tool found (sha256sum, shasum, openssl). Skipping checksum verification."
    return
  fi

  if [ "$actual" != "$expected" ]; then
    error "Checksum verification failed!"
    error "  Expected: $expected"
    error "  Got:      $actual"
    exit 1
  fi

  info "Checksum verified."
}

# installFile extracts the binary and copies it to the install directory.
installFile() {
  local tmp_extract="${TMP_DIR}/extract"
  mkdir -p "$tmp_extract"
  tar xzf "$TMP_FILE" -C "$tmp_extract"

  local binary_path="$tmp_extract/${BINARY_NAME}"
  if [ ! -f "$binary_path" ]; then
    error "Binary '${BINARY_NAME}' not found in archive."
    exit 1
  fi

  info "Installing ${BINARY_NAME} to ${INSTALL_DIR}..."

  # Create install dir if it doesn't exist (for non-root ~/.local/bin installs)
  if [ ! -d "$INSTALL_DIR" ]; then
    if [ "$USE_SUDO" = "true" ] && [ "$(id -u)" -ne 0 ]; then
      sudo mkdir -p "$INSTALL_DIR"
    else
      mkdir -p "$INSTALL_DIR"
    fi
  fi

  runAsRoot cp "$binary_path" "${INSTALL_DIR}/${BINARY_NAME}"
  runAsRoot chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

  info "Installed ${GREEN}${BINARY_NAME}${NC} to ${INSTALL_DIR}/${BINARY_NAME}"
}

# testVersion verifies the installed binary works and warns if it's not on PATH.
testVersion() {
  info "$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null || echo "${BINARY_NAME} installed successfully")"
  if ! command -v "$BINARY_NAME" >/dev/null 2>&1; then
    warn "${BINARY_NAME} is not on your \$PATH."
    warn "Add it with:  export PATH=\"${INSTALL_DIR}:\$PATH\""
  fi
}

# cleanup removes the temp directory.
cleanup() {
  if [ -d "${TMP_DIR:-}" ]; then
    rm -rf "$TMP_DIR"
  fi
}

# fail_trap handles errors and ensures cleanup runs.
fail_trap() {
  local result=$?
  if [ "$result" != "0" ]; then
    error "Installation failed."
  fi
  cleanup
  exit $result
}

# help prints usage information.
help() {
  cat <<EOF
${BINARY_NAME} installer

Usage:
  curl -fsSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/install.sh | bash
  curl -fsSL ... | bash -s -- [OPTIONS]

Options:
  --version, -v <version>    Install a specific version (e.g. v1.0.0)
  --no-sudo                  Install without sudo (default dir: ~/.local/bin)
  --install-dir <path>       Custom install directory
  --help, -h                 Show this help message

Environment variables:
  VERIFY_CHECKSUM=false      Skip SHA256 checksum verification
EOF
}

# --- Main ---

trap fail_trap EXIT

# Parse arguments
while [ $# -gt 0 ]; do
  case "$1" in
    --version|-v)
      shift
      if [ $# -eq 0 ]; then
        error "Please provide a version, e.g. --version v1.0.0"
        exit 1
      fi
      DESIRED_VERSION="$1"
      ;;
    --no-sudo)
      USE_SUDO="false"
      # Default to ~/.local/bin when not using sudo
      if [ "$INSTALL_DIR" = "/usr/local/bin" ]; then
        INSTALL_DIR="${HOME}/.local/bin"
      fi
      ;;
    --install-dir)
      shift
      if [ $# -eq 0 ]; then
        error "Please provide an install directory."
        exit 1
      fi
      INSTALL_DIR="$1"
      ;;
    --help|-h)
      help
      exit 0
      ;;
    *)
      error "Unknown option: $1"
      help
      exit 1
      ;;
  esac
  shift
done

initArch
initOS
verifySupported
getLatestVersion
if ! checkInstalledVersion; then
  if [ "$HAS_PREBUILT" = "true" ]; then
    downloadFile
    verifyChecksum
    installFile
  else
    goInstallFallback
  fi
fi
testVersion
cleanup
trap - EXIT
