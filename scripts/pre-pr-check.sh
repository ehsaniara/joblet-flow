#!/bin/bash
# Pre-PR verification pipeline for joblet-flow.
#
# Run this before opening a PR. It validates the working tree:
#
#   1. Format check   (gofmt)
#   2. Static check    (go vet)
#   3. Dependency hygiene (go mod tidy is a no-op)
#   4. Unit tests (cache disabled)
#   5. Build the engine
#   6. E2E suite: a throwaway engine against the real joblet install
#      (tests/e2e/run_tests.sh; needs joblet installed and running)

set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GREEN='\033[0;32m'; RED='\033[0;31m'; CYAN='\033[0;36m'; NC='\033[0m'

step() {
    echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}

fail() {
    echo -e "\n${RED}❌ PRE-PR CHECK FAILED: $1${NC}"
    exit 1
}

cd "$ROOT"

step "1/6 Format check (gofmt)"
unformatted="$(gofmt -l .)"
[ -z "$unformatted" ] || { echo "$unformatted"; fail "gofmt - run 'make fmt'"; }
echo -e "${GREEN}✓ gofmt clean${NC}"

step "2/6 Static analysis (go vet)"
go vet ./... || fail "go vet"

step "3/6 Dependency hygiene (go mod tidy)"
before="$(cat go.mod go.sum 2>/dev/null | sha256sum)"
go mod tidy || fail "go mod tidy"
after="$(cat go.mod go.sum 2>/dev/null | sha256sum)"
[ "$before" = "$after" ] || fail "go mod tidy changed go.mod/go.sum - commit the tidied result"
echo -e "${GREEN}✓ modules tidy${NC}"

step "4/6 Unit tests"
go test -count=1 ./... || fail "unit tests"

step "5/6 Build (engine)"
make build || fail "build"

step "6/6 E2E suite (engine + real joblet)"
"$ROOT/tests/e2e/run_tests.sh" || fail "e2e suite"

echo -e "\n${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✅ PRE-PR CHECK PASSED${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
