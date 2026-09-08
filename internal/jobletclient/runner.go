// Package jobletclient runs flow job-activities on joblet and dials it over mTLS.
package jobletclient

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"time"

	pb "github.com/ehsaniara/joblet-proto/v2/gen"
	flowpb "github.com/ehsaniara/joblet-proto/v2/gen/flow"
	"google.golang.org/grpc"
)

// terminalStatuses are joblet job states after which no transition occurs (COMPLETED = success).
var terminalStatuses = map[string]bool{
	"COMPLETED": true,
	"FAILED":    true,
	"STOPPED":   true,
	"TIMEOUT":   true,
}

// Runner implements engine.JobRunner over a joblet JobService connection.
type Runner struct {
	jobs         pb.JobServiceClient
	pollInterval time.Duration
	log          *slog.Logger
}

// NewRunner builds a Runner from a live joblet connection.
func NewRunner(conn *grpc.ClientConn, log *slog.Logger) *Runner {
	if log == nil {
		log = slog.Default()
	}
	return &Runner{
		jobs:         pb.NewJobServiceClient(conn),
		pollInterval: 200 * time.Millisecond,
		log:          log,
	}
}

// Run submits the activity's job, waits for terminal status, and captures logs.
func (r *Runner) Run(ctx context.Context, name string, spec *flowpb.FlowJobSpec) (*flowpb.RunActivityResponse, error) {
	started, err := r.jobs.RunJob(ctx, specToRunJob(spec))
	if err != nil {
		return nil, err
	}
	uuid := started.GetJobUuid()

	final, err := r.waitTerminal(ctx, uuid)
	if err != nil {
		return nil, err
	}

	// joblet exposes a single combined log stream; capture it as stdout.
	stdout, err := r.captureLogs(ctx, uuid)
	if err != nil {
		r.log.Warn("failed to capture activity logs", "job_uuid", uuid, "error", err)
	}

	return &flowpb.RunActivityResponse{
		ExitCode: final.GetExitCode(),
		Stdout:   stdout,
		Status:   final.GetStatus(),
		JobUuid:  uuid,
	}, nil
}

// waitTerminal polls job status until it reaches a terminal state or ctx ends.
func (r *Runner) waitTerminal(ctx context.Context, uuid string) (*pb.GetJobStatusResponse, error) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		st, err := r.jobs.GetJobStatus(ctx, &pb.GetJobStatusRequest{Uuid: uuid})
		if err != nil {
			return nil, err
		}
		if terminalStatuses[st.GetStatus()] {
			return st, nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// captureLogs reads the job's full log stream into a string.
func (r *Runner) captureLogs(ctx context.Context, uuid string) (string, error) {
	stream, err := r.jobs.GetJobLogs(ctx, &pb.GetJobLogsRequest{Uuid: uuid})
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return buf.String(), err
		}
		buf.Write(chunk.GetPayload())
	}
	return buf.String(), nil
}

// specToRunJob maps a FlowJobSpec onto a joblet RunJobRequest; the spec's
// Node field is ignored.
func specToRunJob(spec *flowpb.FlowJobSpec) *pb.RunJobRequest {
	req := &pb.RunJobRequest{
		Runtime:     spec.GetRuntime(),
		Command:     spec.GetCommand(),
		Args:        spec.GetArgs(),
		Environment: spec.GetEnv(),
		Volumes:     spec.GetVolumes(),
	}
	if res := spec.GetResources(); res != nil {
		req.MaxCpu = res.GetMaxCpu()
		req.MaxMemory = res.GetMaxMemory()
		req.GpuCount = res.GetGpuCount()
	}
	return req
}
