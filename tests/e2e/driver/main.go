// Command driver is the e2e test client for the flow engine: one subcommand
// per FlowService interaction, output shaped for bash suites. Test-only.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	pb "github.com/ehsaniara/joblet-proto/v2/gen/flow"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func client() (pb.FlowServiceClient, context.Context, context.CancelFunc) {
	addr := os.Getenv("FLOW_ADDR")
	if addr == "" {
		addr = "127.0.0.1:50056"
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fatal("dial %s: %v", addr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	return pb.NewFlowServiceClient(conn), ctx, cancel
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: driver <ping|start|get|poll|complete|fail|run-activity|signal|wait-signal> [flags]")
	}
	cmd, args := os.Args[1], os.Args[2:]
	fc, ctx, cancel := client()
	defer cancel()

	switch cmd {
	case "ping":
		// Any server response, including NotFound, proves the engine is up.
		_, err := fc.GetWorkflow(ctx, &pb.GetWorkflowRequest{WorkflowId: "driver-ping"})
		if status.Code(err) == codes.NotFound || err == nil {
			return
		}
		fatal("engine not reachable: %v", err)

	case "start":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		workflow := fs.String("workflow", "", "workflow name")
		id := fs.String("id", "", "workflow id (optional)")
		input := fs.String("input", "", "input payload")
		_ = fs.Parse(args)
		resp, err := fc.StartWorkflow(ctx, &pb.StartWorkflowRequest{
			Workflow: *workflow, WorkflowId: *id, Input: []byte(*input),
		})
		if err != nil {
			fatal("start: %v", err)
		}
		fmt.Println(resp.GetWorkflowId())

	case "get":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		_ = fs.Parse(args)
		resp, err := fc.GetWorkflow(ctx, &pb.GetWorkflowRequest{WorkflowId: *id})
		if status.Code(err) == codes.NotFound {
			fmt.Println("NOTFOUND")
			return
		}
		if err != nil {
			fatal("get: %v", err)
		}
		fmt.Printf("%s\t%s\n", resp.GetStatus(), resp.GetResult())

	case "poll":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		queue := fs.String("queue", "", "task queue")
		timeout := fs.Duration("timeout", 5*time.Second, "poll deadline")
		_ = fs.Parse(args)
		pollCtx, pollCancel := context.WithTimeout(ctx, *timeout)
		defer pollCancel()
		resp, err := fc.PollTask(pollCtx, &pb.PollTaskRequest{Queue: *queue})
		if err != nil {
			fatal("poll: %v", err)
		}
		if !resp.GetAvailable() {
			fmt.Println("NONE")
			return
		}
		t := resp.GetTask()
		fmt.Printf("%s\t%s\t%s\t%d\n", t.GetType(), t.GetWorkflowId(), t.GetHandler(), t.GetStep())

	case "complete":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		result := fs.String("result", "", "result payload")
		_ = fs.Parse(args)
		if _, err := fc.CompleteTask(ctx, &pb.CompleteTaskRequest{
			WorkflowId: *id, Result: []byte(*result),
		}); err != nil {
			fatal("complete: %v", err)
		}

	case "fail":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		errMsg := fs.String("error", "boom", "failure message")
		_ = fs.Parse(args)
		if _, err := fc.FailTask(ctx, &pb.FailTaskRequest{
			WorkflowId: *id, Error: *errMsg,
		}); err != nil {
			fatal("fail: %v", err)
		}

	case "run-activity":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		step := fs.Int("step", 0, "step index")
		name := fs.String("name", "activity", "activity name")
		command := fs.String("cmd", "", "job command")
		argList := fs.String("args", "", "comma-separated job args")
		attempts := fs.Int("max-attempts", 1, "retry attempts")
		_ = fs.Parse(args)
		req := &pb.RunActivityRequest{
			WorkflowId: *id, Step: int32(*step), Name: *name,
			Job: &pb.FlowJobSpec{Command: *command},
		}
		if *argList != "" {
			req.Job.Args = strings.Split(*argList, ",")
		}
		if *attempts > 1 {
			req.Retry = &pb.RetryPolicy{MaxAttempts: int32(*attempts), BaseMs: 100}
		}
		resp, err := fc.RunActivity(ctx, req)
		if err != nil {
			fatal("run-activity: %v", err)
		}
		// Line 1: status and job uuid; remaining lines: captured stdout.
		fmt.Printf("%s\t%s\n%s", resp.GetStatus(), resp.GetJobUuid(), resp.GetStdout())

	case "signal":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		name := fs.String("name", "", "signal name")
		payload := fs.String("payload", "", "signal payload")
		_ = fs.Parse(args)
		if _, err := fc.SignalWorkflow(ctx, &pb.SignalWorkflowRequest{
			WorkflowId: *id, Name: *name, Payload: []byte(*payload),
		}); err != nil {
			fatal("signal: %v", err)
		}

	case "wait-signal":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		step := fs.Int("step", 0, "step index")
		name := fs.String("name", "", "signal name")
		timeout := fs.Duration("timeout", 10*time.Second, "wait deadline")
		_ = fs.Parse(args)
		waitCtx, waitCancel := context.WithTimeout(ctx, *timeout)
		defer waitCancel()
		resp, err := fc.WaitSignal(waitCtx, &pb.WaitSignalRequest{
			WorkflowId: *id, Step: int32(*step), Name: *name,
		})
		if err != nil {
			fatal("wait-signal: %v", err)
		}
		fmt.Printf("%s\n", resp.GetPayload())

	case "run-worker-activity":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		step := fs.Int("step", 0, "step index")
		name := fs.String("name", "activity", "activity name")
		input := fs.String("input", "", "activity input")
		attempts := fs.Int("max-attempts", 1, "retry attempts")
		_ = fs.Parse(args)
		req := &pb.RunWorkerActivityRequest{
			WorkflowId: *id, Step: int32(*step), Name: *name, Input: []byte(*input),
		}
		if *attempts > 1 {
			req.Retry = &pb.RetryPolicy{MaxAttempts: int32(*attempts), BaseMs: 100}
		}
		resp, err := fc.RunWorkerActivity(ctx, req)
		if err != nil {
			fatal("run-worker-activity: %v", err)
		}
		// Line: result then error (one of them empty).
		fmt.Printf("%s\t%s\n", resp.GetResult(), resp.GetError())

	case "complete-activity":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		step := fs.Int("step", 0, "step index")
		result := fs.String("result", "", "activity result")
		_ = fs.Parse(args)
		if _, err := fc.CompleteActivity(ctx, &pb.CompleteActivityRequest{
			WorkflowId: *id, Step: int32(*step), Result: []byte(*result),
		}); err != nil {
			fatal("complete-activity: %v", err)
		}

	case "fail-activity":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		step := fs.Int("step", 0, "step index")
		errMsg := fs.String("error", "boom", "failure message")
		_ = fs.Parse(args)
		if _, err := fc.FailActivity(ctx, &pb.FailActivityRequest{
			WorkflowId: *id, Step: int32(*step), Error: *errMsg,
		}); err != nil {
			fatal("fail-activity: %v", err)
		}

	case "record-side-effect":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		step := fs.Int("step", 0, "step index")
		data := fs.String("data", "", "side-effect data")
		_ = fs.Parse(args)
		if _, err := fc.RecordSideEffect(ctx, &pb.RecordSideEffectRequest{
			WorkflowId: *id, Step: int32(*step), Data: []byte(*data),
		}); err != nil {
			fatal("record-side-effect: %v", err)
		}

	case "get-side-effect":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		id := fs.String("id", "", "workflow id")
		step := fs.Int("step", 0, "step index")
		_ = fs.Parse(args)
		resp, err := fc.GetSideEffect(ctx, &pb.GetSideEffectRequest{
			WorkflowId: *id, Step: int32(*step),
		})
		if err != nil {
			fatal("get-side-effect: %v", err)
		}
		if !resp.GetFound() {
			fmt.Println("NOTFOUND")
			return
		}
		fmt.Printf("%s\n", resp.GetData())

	default:
		fatal("unknown subcommand %q", cmd)
	}
}
