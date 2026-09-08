#!/bin/bash
# Build the joblet-flow .deb from the working tree.
# Usage: ./scripts/build-deb.sh [arch] [version]

set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ARCH="${1:-$(go env GOARCH)}"
VERSION="${2:-$(git describe --tags --abbrev=0 2>/dev/null || echo "0.1.0")}"
VERSION="${VERSION#v}"
BUILD_DIR="joblet-flow-deb-$ARCH"
DEB="joblet-flow_${VERSION}_${ARCH}.deb"

echo "📦 Building $DEB..."

if [ ! -f "./bin/joblet-flow" ]; then
    echo "Building engine binary..."
    GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build -o bin/joblet-flow ./cmd/joblet-flow
fi

# Verify the binary architecture matches the target package architecture
case "$ARCH" in
    amd64) EXPECTED_ARCH="x86-64" ;;
    arm64) EXPECTED_ARCH="aarch64" ;;
    *) EXPECTED_ARCH="" ;;
esac
if [ -n "$EXPECTED_ARCH" ] && ! file ./bin/joblet-flow | grep -q "$EXPECTED_ARCH"; then
    echo "❌ ./bin/joblet-flow is not $EXPECTED_ARCH but package arch is $ARCH"
    echo "   Rebuild with: rm bin/joblet-flow && ./scripts/build-deb.sh $ARCH"
    exit 1
fi

rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR/DEBIAN" \
    "$BUILD_DIR/opt/joblet-flow/bin" \
    "$BUILD_DIR/etc/systemd/system"

cp ./bin/joblet-flow "$BUILD_DIR/opt/joblet-flow/bin/"
chmod 755 "$BUILD_DIR/opt/joblet-flow/bin/joblet-flow"
cp ./scripts/joblet-flow.service "$BUILD_DIR/etc/systemd/system/"

sed -e "s/VERSION_PLACEHOLDER/$VERSION/" -e "s/ARCH_PLACEHOLDER/$ARCH/" \
    ./debian/control > "$BUILD_DIR/DEBIAN/control"
for script in postinst prerm postrm; do
    cp "./debian/$script" "$BUILD_DIR/DEBIAN/$script"
    chmod 755 "$BUILD_DIR/DEBIAN/$script"
done

dpkg-deb --build --root-owner-group "$BUILD_DIR" "$DEB" >/dev/null
rm -rf "$BUILD_DIR"
echo "✅ Built $DEB"
