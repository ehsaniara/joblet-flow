#!/bin/bash
# Build the joblet-flow .rpm from the working tree.
# Usage: ./scripts/build-rpm.sh [rpm_arch] [version]
#   rpm_arch: x86_64 (default) or aarch64

set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

RPM_ARCH="${1:-x86_64}"
VERSION="${2:-$(git describe --tags --abbrev=0 2>/dev/null || echo "0.1.0")}"
VERSION="${VERSION#v}"
CLEAN_VERSION="${VERSION%%-*}"
PACKAGE_NAME="joblet-flow"
BUILD_DIR="rpmbuild"

case "$RPM_ARCH" in
    x86_64) GOARCH=amd64 ;;
    aarch64) GOARCH=arm64 ;;
    *) echo "❌ unsupported rpm arch: $RPM_ARCH (use x86_64 or aarch64)"; exit 1 ;;
esac

echo "📦 Building ${PACKAGE_NAME}-${CLEAN_VERSION}.${RPM_ARCH}.rpm..."

# Build and verify the engine binary for the target arch
GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=0 go build -o bin/joblet-flow ./cmd/joblet-flow
case "$RPM_ARCH" in
    x86_64) EXPECTED="x86-64" ;;
    aarch64) EXPECTED="aarch64" ;;
esac
if ! file bin/joblet-flow | grep -q "$EXPECTED"; then
    echo "❌ bin/joblet-flow is not $EXPECTED"; exit 1
fi

rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}
SRC="$BUILD_DIR/SOURCES/${PACKAGE_NAME}-${CLEAN_VERSION}"
mkdir -p "$SRC"
cp bin/joblet-flow "$SRC/"
cp scripts/joblet-flow.service "$SRC/"
tar czf "$BUILD_DIR/SOURCES/${PACKAGE_NAME}-${CLEAN_VERSION}.tar.gz" \
    -C "$BUILD_DIR/SOURCES" "${PACKAGE_NAME}-${CLEAN_VERSION}"

cat > "$BUILD_DIR/SPECS/${PACKAGE_NAME}.spec" << EOF
# The binary is a prebuilt, cross-compiled, already-stripped (Go -w -s) static
# binary; skip debuginfo and the host-arch strip/brp post-install steps.
%global debug_package %{nil}
%define _build_id_links none
%global __os_install_post %{nil}

Name:           ${PACKAGE_NAME}
Version:        ${CLEAN_VERSION}
Release:        1%{?dist}
Summary:        Joblet Flow Engine - durable workflow orchestrator
License:        MIT
Source0:        ${PACKAGE_NAME}-${CLEAN_VERSION}.tar.gz
Requires:       systemd

%description
Orchestrates workflows whose activities run as isolated joblet jobs.
Requires a joblet install on the same host; connects to it over mTLS
using the joblet install's client configuration.

%prep
%setup -q

%install
mkdir -p \$RPM_BUILD_ROOT/opt/joblet-flow/bin
mkdir -p \$RPM_BUILD_ROOT/etc/systemd/system
install -m 755 joblet-flow \$RPM_BUILD_ROOT/opt/joblet-flow/bin/joblet-flow
install -m 644 joblet-flow.service \$RPM_BUILD_ROOT/etc/systemd/system/joblet-flow.service

%post
systemctl daemon-reload
systemctl enable joblet-flow.service >/dev/null 2>&1 || true
systemctl restart joblet-flow.service || true
echo ""
echo "✅ joblet-flow installed"
echo "   Engine:  127.0.0.1:50055 (FlowService, loopback only)"
echo "   Status:  systemctl status joblet-flow"

%preun
if [ \$1 -eq 0 ]; then
    systemctl stop joblet-flow.service 2>/dev/null || true
    systemctl disable joblet-flow.service 2>/dev/null || true
fi

%postun
if [ \$1 -eq 0 ]; then
    rm -rf /opt/joblet-flow
    systemctl daemon-reload 2>/dev/null || true
fi

%files
%defattr(-,root,root,-)
/opt/joblet-flow/bin/joblet-flow
/etc/systemd/system/joblet-flow.service

%changelog
* $(date '+%a %b %d %Y') Jay Ehsaniara <ehsaniara@gmail.com> - ${CLEAN_VERSION}-1
- Release ${CLEAN_VERSION}
EOF

rpmbuild --define "_topdir $(pwd)/$BUILD_DIR" \
    --define "_arch $RPM_ARCH" \
    --target "$RPM_ARCH" \
    -bb "$BUILD_DIR/SPECS/${PACKAGE_NAME}.spec"

RPM=$(find "$BUILD_DIR/RPMS" -name "*.rpm" | head -1)
cp "$RPM" "./${PACKAGE_NAME}-${CLEAN_VERSION}-1.${RPM_ARCH}.rpm"
rm -rf "$BUILD_DIR"
echo "✅ Built ${PACKAGE_NAME}-${CLEAN_VERSION}-1.${RPM_ARCH}.rpm"
