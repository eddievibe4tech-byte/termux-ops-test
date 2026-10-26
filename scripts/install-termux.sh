#!/data/data/com.termux/files/usr/bin/bash
# hlops Termux install script
set -euo pipefail

PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
REPO="eddievibe4tech-byte/termux-ops-test"
VERSION="${HLOPS_VERSION:-latest}"

if [ ! -d "$PREFIX/bin" ]; then
  echo "ERROR: this does not look like a Termux environment (no \$PREFIX/bin)." >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "NOTE: golang is not installed. It is only required if you want to build from source."
  echo "      You can install it with: pkg install golang"
fi

echo "提醒：請先執行 'termux-setup-storage' 以取得共享儲存權限（用於 ~/storage/shared 備份）。"

TAG="$VERSION"
if [ "$VERSION" = "latest" ]; then
  TAG=$(curl -sSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | head -1 | cut -d'"' -f4)
fi
if [ -z "$TAG" ]; then
  echo "ERROR: no release found. Building from source instead." >&2
  TMPDIR_SRC=$(mktemp -d); trap 'rm -rf "$TMPDIR_SRC"' EXIT
  git clone --depth 1 "https://github.com/$REPO" "$TMPDIR_SRC"
  (cd "$TMPDIR_SRC" && CGO_ENABLED=0 go build -o "$PREFIX/bin/hlops" ./cmd/hlops)
  echo "Installed hlops (source build) -> $PREFIX/bin/hlops"
  exit 0
fi

ARCH=$(uname -m)
case "$ARCH" in
  aarch64|arm64) GOARCH=arm64 ;;
  x86_64)        GOARCH=amd64 ;;
  *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;;
esac

URL="https://github.com/$REPO/releases/download/$TAG/hlops-linux-$GOARCH"
echo "Downloading $URL ..."
curl -sSLf -o "$PREFIX/bin/hlops" "$URL"
chmod +x "$PREFIX/bin/hlops"
echo "Installed hlops $TAG -> $PREFIX/bin/hlops"
hlops version
