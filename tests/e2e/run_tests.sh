#!/bin/bash
# E2E runner for joblet-flow. Builds the engine and the test driver from the
# working tree, starts a throwaway engine wired over mTLS to the host's real
# joblet install, runs every tests/*.sh against it, then tears down.
#
# Requires an installed, running joblet (the engine dials it with the default
# rnx-config.yml node). Fail fast: the first failing suite stops the run.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
source "$SCRIPT_DIR/lib/test_framework.sh"

ENGINE_BIN="$REPO_ROOT/bin/joblet-flow"
ENGINE_LOG="$(mktemp)"
ENGINE_PID=""

cleanup() {
    [[ -n "$ENGINE_PID" ]] && kill "$ENGINE_PID" 2>/dev/null
    rm -f "$ENGINE_LOG"
}
trap cleanup EXIT

echo -e "${CYAN}Building engine and test driver...${NC}"
if ! (cd "$REPO_ROOT" && go build -o bin/joblet-flow ./cmd/joblet-flow &&
        go build -o bin/flow-e2e-driver ./tests/e2e/driver); then
    echo -e "${RED}Build failed${NC}"; exit 1
fi

echo -e "${CYAN}Checking the joblet install (activities run as real joblet jobs)...${NC}"
if ! "$RNX_BINARY" job list >/dev/null 2>&1; then
    echo -e "${RED}joblet is not reachable via $RNX_BINARY - install and start joblet first${NC}"
    exit 1
fi
echo -e "${GREEN}✓ joblet reachable${NC}"

echo -e "${CYAN}Starting engine on $FLOW_ADDR...${NC}"
FLOW_LISTEN_ADDR="$FLOW_ADDR" "$ENGINE_BIN" >"$ENGINE_LOG" 2>&1 &
ENGINE_PID=$!

ready=false
for _ in $(seq 1 25); do
    if "$DRIVER" ping 2>/dev/null; then ready=true; break; fi
    sleep 0.2
done
if [[ "$ready" != true ]]; then
    echo -e "${RED}Engine did not become ready. Log:${NC}"; cat "$ENGINE_LOG"; exit 1
fi
echo -e "${GREEN}✓ Engine ready (pid $ENGINE_PID)${NC}"

# Discover and run suites; stop at the first failure
suites=0; passed=0
for test_file in $(ls "$SCRIPT_DIR"/tests/*.sh 2>/dev/null | sort); do
    name="$(basename "$test_file" .sh)"
    suites=$((suites + 1))
    echo -e "\n${CYAN}━━━ Running: $name ━━━${NC}"
    chmod +x "$test_file"
    if "$test_file"; then
        passed=$((passed + 1))
    else
        echo -e "\n${RED}✗ Fail fast: stopping after failure in $name${NC}"
        echo -e "${RED}━━━ E2E FAILED ($passed/$suites suites passed before the failure) ━━━${NC}"
        exit 1
    fi
done

echo ""
echo -e "${GREEN}━━━ ALL $passed E2E SUITES PASSED ━━━${NC}"
