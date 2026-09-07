#!/bin/bash
# E2E: signal delivery, buffering, and wait memoization.

source "$(dirname "$0")/../lib/test_framework.sh"
test_suite_init "Signal Tests"

WF_ID=""

test_section "Buffered Signal"

test_buffered_signal() {
    WF_ID=$("$DRIVER" start --workflow signal-demo) || return 1
    "$DRIVER" signal --id "$WF_ID" --name go --payload early || return 1
    local got
    got=$("$DRIVER" wait-signal --id "$WF_ID" --step 1 --name go --timeout 5s) || return 1
    [[ "$got" == "early" ]] || { echo "    ✗ payload: $got"; return 1; }
    echo "    Signal sent before the wait was buffered and delivered"
}
run_test "A signal sent before WaitSignal is buffered" test_buffered_signal

test_section "Live Signal"

test_live_signal() {
    local out_file
    out_file=$(mktemp)
    "$DRIVER" wait-signal --id "$WF_ID" --step 2 --name live --timeout 10s > "$out_file" &
    local waiter=$!
    sleep 1
    "$DRIVER" signal --id "$WF_ID" --name live --payload delivered || return 1
    wait "$waiter" || { rm -f "$out_file"; echo "    ✗ waiter failed"; return 1; }
    local got
    got=$(cat "$out_file"); rm -f "$out_file"
    [[ "$got" == "delivered" ]] || { echo "    ✗ payload: $got"; return 1; }
    echo "    Blocked waiter received the live signal"
}
run_test "WaitSignal receives a signal sent while blocked" test_live_signal

test_section "Wait Memoization"

test_wait_memoized() {
    local got
    got=$("$DRIVER" wait-signal --id "$WF_ID" --step 2 --name live --timeout 3s) || return 1
    [[ "$got" == "delivered" ]] || { echo "    ✗ replay payload: $got"; return 1; }
    echo "    Replayed wait returned the memoized payload without blocking"
}
run_test "Replayed WaitSignal is memoized by step" test_wait_memoized

test_suite_summary
