#!/bin/bash
# E2E: worker activities (the Temporal-style in-worker activity) and side
# effects, driven through the real FlowService against the installed engine.

source "$(dirname "$0")/../lib/test_framework.sh"
test_suite_init "Worker Activity & Side Effect Tests"

WF_ID="wa-e2e-$$"

test_section "Worker Activity"

# RunWorkerActivity blocks until an activity worker reports the result, so the
# call runs in the background while a second driver invocation completes it,
# mirroring how an SDK activity worker polls and reports.
test_worker_activity_complete() {
    local out_file
    out_file=$(mktemp)
    "$DRIVER" run-worker-activity --id "$WF_ID" --step 0 --name summarize --input in \
        > "$out_file" 2>&1 &
    local waiter=$!

    # Give the engine a moment to enqueue and register the pending call, then
    # poll the activity task and complete it.
    sleep 1
    "$DRIVER" poll --queue default --timeout 5s >/dev/null || { kill $waiter 2>/dev/null; echo "    ✗ no activity task"; rm -f "$out_file"; return 1; }
    "$DRIVER" complete-activity --id "$WF_ID" --step 0 --result out || { rm -f "$out_file"; return 1; }

    wait "$waiter" || { echo "    ✗ RunWorkerActivity failed"; cat "$out_file"; rm -f "$out_file"; return 1; }
    local result
    result=$(cut -f1 "$out_file"); rm -f "$out_file"
    if [[ "$result" != "out" ]]; then
        echo "    ✗ result = '$result', want 'out'"; return 1
    fi
    echo "    Worker activity dispatched, completed by a worker, result returned"
}
run_test "RunWorkerActivity completes via CompleteActivity" test_worker_activity_complete

test_worker_activity_memoized() {
    # Replaying the same step returns the memoized result without a new task.
    local out
    out=$("$DRIVER" run-worker-activity --id "$WF_ID" --step 0 --name summarize --input in)
    if [[ "$(echo "$out" | cut -f1)" != "out" ]]; then
        echo "    ✗ replay result = '$out', want 'out'"; return 1
    fi
    echo "    Replay returned the memoized result, no re-dispatch"
}
run_test "Worker activity replay is memoized by step" test_worker_activity_memoized

test_worker_activity_retry_then_fail() {
    local wf="wa-retry-$$" out_file
    out_file=$(mktemp)
    "$DRIVER" run-worker-activity --id "$wf" --step 0 --name flaky --max-attempts 2 \
        > "$out_file" 2>&1 &
    local waiter=$!
    sleep 1
    "$DRIVER" poll --timeout 5s >/dev/null           # attempt 1
    "$DRIVER" fail-activity --id "$wf" --step 0 --error boom
    "$DRIVER" poll --timeout 5s >/dev/null           # attempt 2 (re-dispatched)
    "$DRIVER" fail-activity --id "$wf" --step 0 --error boom-final
    wait "$waiter" || true
    local line errcol
    line=$(head -1 "$out_file"); rm -f "$out_file"
    errcol=$(printf '%s' "$line" | cut -f2)
    # The clean path returns a RunWorkerActivityResponse with Error set to the
    # last failure, not a transport error from the call timing out.
    if [[ "$errcol" != "boom-final" ]]; then
        echo "    ✗ expected error 'boom-final', got: $line"; return 1
    fi
    echo "    Worker activity retried twice then reported failure cleanly"
}
run_test "Worker activity retries per policy, then fails" test_worker_activity_retry_then_fail

test_section "Side Effects"

test_side_effect_record_and_get() {
    local wf="se-e2e-$$"
    "$DRIVER" record-side-effect --id "$wf" --step 0 --data "llm-42" || return 1
    local got
    got=$("$DRIVER" get-side-effect --id "$wf" --step 0) || return 1
    if [[ "$got" != "llm-42" ]]; then
        echo "    ✗ got '$got', want 'llm-42'"; return 1
    fi
    echo "    Side effect recorded and read back"
}
run_test "RecordSideEffect then GetSideEffect returns the value" test_side_effect_record_and_get

test_side_effect_missing() {
    local got
    got=$("$DRIVER" get-side-effect --id "no-such-wf" --step 9) || return 1
    if [[ "$got" != "NOTFOUND" ]]; then
        echo "    ✗ expected NOTFOUND, got '$got'"; return 1
    fi
    echo "    Unrecorded step reports not-found"
}
run_test "GetSideEffect reports not-found for an unrecorded step" test_side_effect_missing

test_suite_summary
