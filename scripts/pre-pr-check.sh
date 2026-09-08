#!/bin/bash

# Pre-PR verification pipeline
#
# Run this before opening a PR. It validates the working tree end to end on
# this machine's architecture:
#
#   1. Run unit tests (make test)
#   2. Run the e2e suite via tests/e2e/run_tests.sh, which uninstalls
#      joblet-flow AND joblet completely, installs the latest released
#      joblet from GitHub, installs joblet-flow from the working tree as a
#      .deb, and runs every suite against that clean install
#
# Must run in a real terminal - several steps use sudo.

set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ARCH=$(go env GOARCH)

GREEN='\033[0;32m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

step() {
    echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}

fail() {
    echo -e "\n${RED}❌ PRE-PR CHECK FAILED: $1${NC}"
    exit 1
}

if [ ! -t 0 ]; then
    echo "⚠️  No terminal detected - sudo prompts will fail. Run this in a real terminal."
fi

step "1/2 Unit tests"
make -C "$ROOT" test || fail "unit tests"

step "2/2 E2E suite on a clean install ($ARCH)"
"$ROOT/tests/e2e/run_tests.sh" || fail "e2e suite"

echo -e "\n${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  ✅ PRE-PR CHECK PASSED${NC}"
echo -e "${GREEN}  Unit tests + e2e on a clean install verified on $(uname -m)/$(. /etc/os-release && echo "$ID $VERSION_ID")${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
