#!/bin/bash
# E2E: job activities run as real joblet jobs, cross-checked from joblet's
# side via rnx, with step memoization and retry.

source "$(dirname "$0")/../lib/test_framework.sh"
test_suite_init "Job Activity Tests"

WF_ID=""
JOB_UUID=""
MARKER="flow-e2e-$$"
CLEANUP_UUIDS=()

test_section "Activity Execution"

test_activity_runs_real_job() {
    WF_ID=$("$DRIVER" start --workflow activity-demo) || return 1
    "$DRIVER" poll --timeout 5s >/dev/null || return 1
    local out
    out=$("$DRIVER" run-activity --id "$WF_ID" --step 1 --name echo \
        --cmd echo --args "$MARKER-ok") || return 1
    local status uuid
    status=$(echo "$out" | head -1 | cut -f1)
    uuid=$(echo "$out" | head -1 | cut -f2)
    [[ "$status" == "COMPLETED" && -n "$uuid" ]] || { echo "    ✗ $out"; return 1; }
    echo "$out" | grep -q "$MARKER-ok" || { echo "    ✗ output not captured"; return 1; }
    JOB_UUID="$uuid"
    CLEANUP_UUIDS+=("$uuid")
    echo "    Activity COMPLETED as joblet job $uuid, output captured"
}
run_test "RunActivity executes and captures output" test_activity_runs_real_job

test_job_visible_in_joblet() {
    local st
    st=$("$RNX_BINARY" job status "$JOB_UUID" 2>&1) || { echo "    ✗ $st"; return 1; }
    echo "$st" | grep -q "COMPLETED" || { echo "    ✗ joblet disagrees: $st"; return 1; }
    echo "    joblet confirms job $JOB_UUID COMPLETED"
}
run_test "joblet confirms the activity job from its side" test_job_visible_in_joblet

test_section "Memoization"

test_step_replay_memoized() {
    local out uuid
    out=$("$DRIVER" run-activity --id "$WF_ID" --step 1 --name echo \
        --cmd echo --args "should-not-run") || return 1
    uuid=$(echo "$out" | head -1 | cut -f2)
    [[ "$uuid" == "$JOB_UUID" ]] || { echo "    ✗ replay ran a new job: $uuid"; return 1; }
    local count
    count=$("$RNX_BINARY" job list 2>/dev/null | grep -c "$MARKER-ok")
    [[ "$count" == "1" ]] || { echo "    ✗ joblet shows $count jobs for the step"; return 1; }
    echo "    Replay returned the memoized job; joblet shows exactly one job"
}
run_test "Step replay is memoized, no second job runs" test_step_replay_memoized

test_section "Retry"

test_failed_activity_retries() {
    local out status
    out=$("$DRIVER" run-activity --id "$WF_ID" --step 2 --name flaky \
        --cmd sh --args "-c,echo $MARKER-retry; exit 1" --max-attempts 2) || return 1
    status=$(echo "$out" | head -1 | cut -f1)
    [[ "$status" == "FAILED" ]] || { echo "    ✗ expected FAILED, got $status"; return 1; }
    local count
    count=$("$RNX_BINARY" job list 2>/dev/null | grep -c "$MARKER-retry")
    [[ "$count" == "2" ]] || { echo "    ✗ joblet shows $count attempts, expected 2"; return 1; }
    echo "    Activity FAILED after 2 real attempts (confirmed by joblet)"
}
run_test "Failed activity retries per policy, then reports FAILED" test_failed_activity_retries

test_section "Cleanup"

test_cleanup_jobs() {
    local uuid
    for uuid in $("$RNX_BINARY" job list 2>/dev/null | grep "$MARKER" | awk '{print $1}'); do
        "$RNX_BINARY" job delete "$uuid" >/dev/null 2>&1 || true
    done
    echo "    Activity jobs removed from joblet"
}
run_test "Activity jobs cleaned up" test_cleanup_jobs

test_suite_summary
