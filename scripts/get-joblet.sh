#!/bin/bash
# Download the latest released joblet .deb from GitHub for this host's
# architecture and print its local path. Used by the e2e runner so flow is
# always verified against the joblet a user would install.

set -e

case "$(uname -m)" in
    x86_64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) echo "❌ unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

DEST_DIR="${1:-$(mktemp -d)}"
URL=$(curl -fsSL https://api.github.com/repos/ehsaniara/joblet/releases/latest |
    grep '"browser_download_url"' | grep "_${ARCH}\.deb" | head -1 | cut -d '"' -f 4)
[ -n "$URL" ] || { echo "❌ no joblet ${ARCH} .deb in the latest release" >&2; exit 1; }

DEB="$DEST_DIR/$(basename "$URL")"
echo "Downloading $(basename "$URL")..." >&2
curl -fsSL -o "$DEB" "$URL"
echo "$DEB"
