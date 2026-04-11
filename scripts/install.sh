#!/bin/sh

set -eu

REPO_OWNER="aliefe04"
REPO_NAME="portico"
BINARY_NAME="portico"
VERSION=""
INSTALL_DIR=""
SYSTEM_INSTALL=0

usage() {
  cat <<'EOF'
Install Portico from GitHub Releases.

Options:
  --version <tag>       Install a specific version, for example v0.1.0
  --install-dir <path>  Install into a custom directory
  --system              Install system-wide into /usr/local/bin or /opt/homebrew/bin
  --help                Show this help message
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    --system)
      SYSTEM_INSTALL=1
      shift
      ;;
    --help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

normalize_version() {
  case "$1" in
    v*) printf '%s' "$1" ;;
    *) printf 'v%s' "$1" ;;
  esac
}

detect_os() {
  case "$(uname -s)" in
    Darwin) printf 'darwin' ;;
    Linux) printf 'linux' ;;
    *) echo "Unsupported operating system: $(uname -s)" >&2; exit 1 ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac
}

latest_version() {
  curl -fsSL "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" |
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
    return
  fi
  echo "sha256sum or shasum is required" >&2
  exit 1
}

OS_NAME="$(detect_os)"
ARCH_NAME="$(detect_arch)"

if [ -z "$VERSION" ]; then
  VERSION="$(latest_version)"
fi
VERSION="$(normalize_version "$VERSION")"

if [ -z "$INSTALL_DIR" ]; then
  if [ "$SYSTEM_INSTALL" -eq 1 ]; then
    if [ "$OS_NAME" = "darwin" ] && [ -d "/opt/homebrew/bin" ]; then
      INSTALL_DIR="/opt/homebrew/bin"
    else
      INSTALL_DIR="/usr/local/bin"
    fi
  else
    INSTALL_DIR="${HOME}/.local/bin"
  fi
fi

BASE_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${VERSION}"
CHECKSUMS_URL="${BASE_URL}/checksums.txt"
VERSION_TRIMMED="${VERSION#v}"
ARCHIVE_CANDIDATES="${BINARY_NAME}_${OS_NAME}_${ARCH_NAME}.tar.gz ${BINARY_NAME}_${VERSION_TRIMMED}_${OS_NAME}_${ARCH_NAME}.tar.gz"

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

curl -fsSL "$CHECKSUMS_URL" -o "$TMP_DIR/checksums.txt"

ARCHIVE_NAME=""
for candidate in $ARCHIVE_CANDIDATES; do
  if curl -fsSL "${BASE_URL}/${candidate}" -o "$TMP_DIR/$candidate" 2>/dev/null; then
    ARCHIVE_NAME="$candidate"
    break
  fi
done

if [ -z "$ARCHIVE_NAME" ]; then
  echo "Release archive for ${OS_NAME}/${ARCH_NAME} was not found" >&2
  exit 1
fi

EXPECTED_SUM="$(awk -v file="$ARCHIVE_NAME" '$2 == file {print $1}' "$TMP_DIR/checksums.txt")"
if [ -z "$EXPECTED_SUM" ]; then
  echo "Checksum for $ARCHIVE_NAME not found" >&2
  exit 1
fi

ACTUAL_SUM="$(sha256_file "$TMP_DIR/$ARCHIVE_NAME")"
if [ "$EXPECTED_SUM" != "$ACTUAL_SUM" ]; then
  echo "Checksum mismatch for $ARCHIVE_NAME" >&2
  exit 1
fi

tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR" "$BINARY_NAME"

SUDO=""
if [ ! -d "$INSTALL_DIR" ]; then
  if [ "$SYSTEM_INSTALL" -eq 1 ] && [ "$(id -u)" -ne 0 ]; then
    SUDO="sudo"
    $SUDO mkdir -p "$INSTALL_DIR"
  else
    mkdir -p "$INSTALL_DIR"
  fi
fi

if [ "$SYSTEM_INSTALL" -eq 1 ] && [ ! -w "$INSTALL_DIR" ] && [ "$(id -u)" -ne 0 ]; then
  SUDO="sudo"
fi

TARGET_PATH="${INSTALL_DIR}/${BINARY_NAME}"
TMP_TARGET="${TARGET_PATH}.tmp.$$"

if [ -f "$TARGET_PATH" ]; then
  BACKUP_PATH="${TARGET_PATH}.bak.$(date -u +%Y%m%dT%H%M%SZ)"
  if [ -n "$SUDO" ]; then
    $SUDO cp "$TARGET_PATH" "$BACKUP_PATH"
  else
    cp "$TARGET_PATH" "$BACKUP_PATH"
  fi
fi

if [ -n "$SUDO" ]; then
  $SUDO install -m 0755 "$TMP_DIR/$BINARY_NAME" "$TMP_TARGET"
  $SUDO mv "$TMP_TARGET" "$TARGET_PATH"
else
  install -m 0755 "$TMP_DIR/$BINARY_NAME" "$TMP_TARGET"
  mv "$TMP_TARGET" "$TARGET_PATH"
fi

echo "Installed Portico ${VERSION} to ${TARGET_PATH}"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    if [ "$SYSTEM_INSTALL" -eq 0 ]; then
      echo "Add ${INSTALL_DIR} to your PATH if needed."
    fi
    ;;
esac
