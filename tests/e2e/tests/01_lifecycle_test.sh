#!/bin/bash
# E2E: workflow lifecycle through the public FlowService surface.

source "$(dirname "$0")/../lib/test_framework.sh"
test_suite_init "Workflow Lifecycle Tests"

WF_ID=""

test_section "Start and Task Handout"

test_start_returns_id() {
    WF_ID=$("$DRIVER" start --workflow demo --input hello) || return 1
    [[ -n "$WF_ID" ]] || { echo "    ✗ empty workflow id"; return 1; }
    echo "    Workflow id: $WF_ID"
}
run_test "StartWorkflow returns a workflow id" test_start_returns_id

test_poll_hands_out_task() {
    local line
    line=$("$DRIVER" poll --timeout 5s) || return 1
    if [[ "$line" == "NONE" ]]; then
        echo "    ✗ no task available"; return 1
    fi
    echo "$line" | grep -q "$WF_ID" || { echo "    ✗ task is not for $WF_ID: $line"; return 1; }
    echo "$line" | grep -q "demo" || { echo "    ✗ task handler is not demo: $line"; return 1; }
    echo "    Task handed out: $line"
}
run_test "PollTask hands out the workflow task" test_poll_hands_out_task

test_section "Idempotent Start"

test_idempotent_start() {
    local again
    again=$("$DRIVER" start --workflow demo --id "$WF_ID") || return 1
    [[ "$again" == "$WF_ID" ]] || { echo "    ✗ id changed: $again"; return 1; }
    local task
    task=$("$DRIVER" poll --timeout 2s) || return 1
    [[ "$task" == "NONE" ]] || { echo "    ✗ duplicate task enqueued: $task"; return 1; }
    echo "    Same id returned, no duplicate task"
}
run_test "StartWorkflow is idempotent on workflow_id" test_idempotent_start

test_section "Completion and Status"

test_complete_and_get() {
    "$DRIVER" complete --id "$WF_ID" --result done || return 1
    local got
    got=$("$DRIVER" get --id "$WF_ID") || return 1
    [[ "$got" == "COMPLETED"*"done" ]] || { echo "    ✗ unexpected state: $got"; return 1; }
    echo "    Status: $got"
}
run_test "CompleteTask marks the run COMPLETED with its result" test_complete_and_get

test_failed_workflow() {
    local id got
    id=$("$DRIVER" start --workflow doomed) || return 1
    "$DRIVER" poll --timeout 5s >/dev/null || return 1
    "$DRIVER" fail --id "$id" --error "handler exploded" || return 1
    got=$("$DRIVER" get --id "$id") || return 1
    [[ "$got" == "FAILED"* ]] || { echo "    ✗ unexpected state: $got"; return 1; }
    echo "    Status: $got"
}
run_test "FailTask marks the run FAILED" test_failed_workflow

test_unknown_workflow() {
    local got
    got=$("$DRIVER" get --id no-such-workflow) || return 1
    [[ "$got" == "NOTFOUND" ]] || { echo "    ✗ expected NOTFOUND, got: $got"; return 1; }
    echo "    Unknown id returns NotFound"
}
run_test "GetWorkflow returns NotFound for unknown ids" test_unknown_workflow

test_suite_summary
