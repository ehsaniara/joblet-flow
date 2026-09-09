#!/bin/bash
# E2E: durable state. The installed engine supervises the flow-store subprocess,
# which persists every state mutation to an on-disk event log. Drive a workflow
# and assert the events landed on disk (root-owned, so reads use sudo).

source "$(dirname "$0")/../lib/test_framework.sh"
test_suite_init "Durable State Tests"

STATE_LOG="/opt/joblet-flow/state/events.log"
WF_ID=""

test_section "flow-store Subprocess"

test_flowstore_running() {
    if ! pgrep -f '/opt/joblet-flow/bin/flow-store' >/dev/null; then
        echo "    ✗ flow-store subprocess not running"
        return 1
    fi
    echo "    flow-store subprocess is running"
}
run_test "flow-store runs under the engine" test_flowstore_running

test_socket_exists() {
    if ! sudo test -S /opt/joblet-flow/run/flow-store.sock; then
        echo "    ✗ flow-store socket missing"
        return 1
    fi
    echo "    flow-store IPC socket present"
}
run_test "flow-store IPC socket exists" test_socket_exists

test_section "State Persistence"

test_run_workflow_persists() {
    local before after
    before=$(sudo stat -c %s "$STATE_LOG" 2>/dev/null || echo 0)

    WF_ID=$("$DRIVER" start --workflow demo --input hi) || return 1
    "$DRIVER" poll --timeout 5s >/dev/null || return 1
    "$DRIVER" run-activity --id "$WF_ID" --step 1 --name echo \
        --cmd echo --args state-e2e-ok >/dev/null || return 1
    "$DRIVER" complete --id "$WF_ID" --result done || return 1

    # The shipper is async; wait briefly for the event log to grow.
    local i
    for i in $(seq 1 20); do
        after=$(sudo stat -c %s "$STATE_LOG" 2>/dev/null || echo 0)
        if [[ "$after" -gt "$before" ]]; then
            echo "    Event log grew ${before} -> ${after} bytes after the workflow"
            return 0
        fi
        sleep 0.5
    done
    echo "    ✗ event log did not grow (before=$before after=$after)"
    return 1
}
run_test "Running a workflow persists events to disk" test_run_workflow_persists

test_log_root_only() {
    local mode_owner
    mode_owner=$(sudo stat -c '%a %U' "$STATE_LOG" 2>/dev/null)
    if [[ "$mode_owner" != "600 root" ]]; then
        echo "    ✗ $STATE_LOG is '$mode_owner', expected '600 root'"
        return 1
    fi
    echo "    Event log is 600 root"
}
run_test "Event log is root-only" test_log_root_only

test_suite_summary
