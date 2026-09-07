#!/bin/bash
# Minimal e2e test framework for joblet-flow, modeled on the joblet e2e suite.
# Tests drive the test driver binary against a running engine backed by a real
# joblet install, and assert real outcomes. Sourced by each tests/*.sh.

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

# Binaries / connection (overridable by the runner or the environment)
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
DRIVER="${DRIVER:-$REPO_ROOT/bin/flow-e2e-driver}"
export FLOW_ADDR="${FLOW_ADDR:-127.0.0.1:50056}"
# The client the joblet install provides, used to cross-check activity jobs
# from joblet's side
RNX_BINARY="${RNX_BINARY:-/usr/local/bin/rnx}"

# Counters
TOTAL_TESTS=0; PASSED_TESTS=0; FAILED_TESTS=0; SKIPPED_TESTS=0
SUITE_NAME=""

test_suite_init() {
    SUITE_NAME="$1"
    TOTAL_TESTS=0; PASSED_TESTS=0; FAILED_TESTS=0; SKIPPED_TESTS=0
    echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}  $SUITE_NAME${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}Started: $(date '+%Y-%m-%d %H:%M:%S')${NC}"
}

test_section() {
    echo -e "\n${YELLOW}▶ $1${NC}"
    echo -e "${BLUE}$(printf '─%.0s' {1..65})${NC}"
}

run_test() {
    local name="$1"; local fn="$2"
    ((TOTAL_TESTS++))
    echo -e "\n${BLUE}[$TOTAL_TESTS] Testing: $name${NC}"
    if $fn; then
        ((PASSED_TESTS++)); echo -e "${GREEN}  ✓ PASS${NC}: $name"; return 0
    else
        ((FAILED_TESTS++)); echo -e "${RED}  ✗ FAIL${NC}: $name"
        # Fail fast: one failure is enough, stop the suite immediately
        echo -e "${RED}  Aborting suite after first failure (${PASSED_TESTS} passed before it)${NC}"
        exit 1
    fi
}

test_suite_summary() {
    echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "  Total:   $TOTAL_TESTS"
    echo -e "  ${GREEN}Passed:  $PASSED_TESTS${NC}"
    echo -e "  ${RED}Failed:  $FAILED_TESTS${NC}"
    if [[ $FAILED_TESTS -eq 0 && $TOTAL_TESTS -gt 0 ]]; then
        echo -e "\n${GREEN}✅ SUITE PASSED: ${SUITE_NAME}${NC}"
        return 0
    fi
    echo -e "\n${RED}❌ SUITE FAILED: ${SUITE_NAME}${NC}"
    return 1
}
