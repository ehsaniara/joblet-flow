#!/bin/bash
# E2E runner for joblet-flow. Uninstalls joblet-flow AND joblet completely,
# installs the latest released joblet from GitHub, installs joblet-flow from
# the working tree as a .deb, then runs every tests/*.sh against the
# installed service (needs sudo).
#
# Fail fast: the first failing suite stops the run.
# ASSUME_YES=0 asks before uninstalling instead of assuming yes.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# The installed flow service listens on loopback :50055
export FLOW_ADDR="${FLOW_ADDR:-127.0.0.1:50055}"
source "$SCRIPT_DIR/lib/test_framework.sh"

banner() {
    echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

cd "$REPO_ROOT"

banner "Clean Install (purge + released joblet + flow from working tree)"

echo -e "${YELLOW}⚠️  This removes joblet-flow AND joblet from this host, including data.${NC}"
if [[ "${ASSUME_YES:-1}" == "1" ]]; then
    echo "Automated run: assuming yes."
else
    read -r -p "Continue? [y/N] " answer
    case "$answer" in y|Y|yes|YES) ;; *) echo "Aborted."; exit 1 ;; esac
fi

echo -e "${BLUE}Purging existing joblet-flow and joblet installations...${NC}"
sudo systemctl stop joblet-flow.service 2>/dev/null || true
sudo dpkg --purge joblet-flow 2>/dev/null || true
sudo rm -rf /opt/joblet-flow
if dpkg -s joblet >/dev/null 2>&1; then
    sudo dpkg --purge joblet || { echo -e "${RED}joblet purge failed${NC}"; exit 1; }
fi
# dpkg purge leaves the bridge behind; remove it so no joblet trace remains
sudo ip link delete joblet0 2>/dev/null || true
echo -e "${GREEN}✓ Host clean${NC}"

echo -e "${BLUE}Installing latest released joblet from GitHub...${NC}"
JOBLET_DEB=$(./scripts/get-joblet.sh) || exit 1
sudo DEBIAN_FRONTEND=noninteractive dpkg -i "$JOBLET_DEB" || { echo -e "${RED}joblet install failed${NC}"; exit 1; }
sudo systemctl start joblet.service
ready=false
for _ in $(seq 1 60); do
    if "$RNX_BINARY" job list >/dev/null 2>&1; then ready=true; break; fi
    sleep 0.5
done
[[ "$ready" == true ]] || { echo -e "${RED}joblet not ready - check: journalctl -u joblet${NC}"; exit 1; }
echo -e "${GREEN}✓ joblet $(basename "$JOBLET_DEB") installed and serving${NC}"

echo -e "${BLUE}Building and installing joblet-flow from the working tree...${NC}"
rm -f bin/joblet-flow joblet-flow_*.deb
go build -o bin/joblet-flow ./cmd/joblet-flow || exit 1
go build -o bin/flow-e2e-driver ./tests/e2e/driver || exit 1
./scripts/build-deb.sh || exit 1
FLOW_DEB=$(ls -t joblet-flow_*.deb | head -1)
sudo dpkg -i "$FLOW_DEB" || { echo -e "${RED}flow install failed${NC}"; exit 1; }
ready=false
for _ in $(seq 1 25); do
    if "$DRIVER" ping 2>/dev/null; then ready=true; break; fi
    sleep 0.2
done
[[ "$ready" == true ]] || { echo -e "${RED}flow engine not ready - check: journalctl -u joblet-flow${NC}"; exit 1; }
echo -e "${GREEN}✓ Clean install ready (flow serving on $FLOW_ADDR)${NC}"

banner "Joblet-Flow E2E Test Suite"
TESTS_TO_RUN=($(ls "$SCRIPT_DIR"/tests/*.sh 2>/dev/null | sort))
total=${#TESTS_TO_RUN[@]}
echo -e "${BLUE}Found $total test suites to run${NC}\n"

# Fail fast: stop at the first failing suite instead of running the rest
passed=0; failed=0
for test_file in "${TESTS_TO_RUN[@]}"; do
    name="$(basename "$test_file" .sh)"
    banner "Running: $name"
    chmod +x "$test_file"
    if "$test_file"; then
        echo -e "${GREEN}✓ $name completed successfully${NC}"
        passed=$((passed + 1))
    else
        failed=1
        echo -e "\n${RED}✗ Fail fast: stopping after failure in $name${NC}"
        break
    fi
done
skipped=$((total - passed - failed))

banner "Overall Test Summary"
echo -e "Test Suites Found:  $total"
echo -e "Suites Passed:      ${GREEN}$passed${NC}"
echo -e "Suites Failed:      ${RED}$failed${NC}"
if [[ $skipped -gt 0 ]]; then
    echo -e "Suites Not Run:     ${YELLOW}$skipped${NC} (stopped at first failure)"
fi

if [[ $failed -eq 0 ]]; then
    echo -e "\n${GREEN}🎉 ALL TEST SUITES PASSED!${NC}"
    echo -e "${GREEN}joblet-flow is working correctly.${NC}"
    exit 0
fi
echo -e "\n${RED}❌ SOME TEST SUITES FAILED${NC}"
echo -e "${RED}Please review the failures above.${NC}"
exit 1
