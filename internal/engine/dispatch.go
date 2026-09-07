package engine

import (
	"context"
	"time"

	pb "github.com/ehsaniara/joblet-proto/v2/gen/flow"
)

// JobRunner dispatches an activity's joblet job (implemented by package jobletclient).
type JobRunner interface {
	Run(ctx context.Context, name string, spec *pb.FlowJobSpec) (*pb.RunActivityResponse, error)
}

const (
	defaultBackoffBase = 100 * time.Millisecond
	maxBackoffShift    = 20 // cap exponential growth to avoid overflow
)

// runWithRetry dispatches a job activity, retrying non-COMPLETED outcomes per the policy.
func runWithRetry(ctx context.Context, runner JobRunner, name string, spec *pb.FlowJobSpec, rp *pb.RetryPolicy) (*pb.RunActivityResponse, error) {
	maxAttempts := int32(1)
	if rp != nil && rp.MaxAttempts > 1 {
		maxAttempts = rp.MaxAttempts
	}

	var last *pb.RunActivityResponse
	var err error
	for attempt := int32(1); attempt <= maxAttempts; attempt++ {
		last, err = runner.Run(ctx, name, spec)
		if err == nil && last != nil && last.Status == StatusCompleted {
			return last, nil
		}
		if attempt < maxAttempts {
			select {
			case <-time.After(backoff(rp, attempt)):
			case <-ctx.Done():
				return last, ctx.Err()
			}
		}
	}
	if last == nil {
		return nil, err
	}
	return last, nil
}

// backoff returns the delay before a retry attempt (fixed, or exponential by default).
func backoff(rp *pb.RetryPolicy, attempt int32) time.Duration {
	base := defaultBackoffBase
	if rp != nil && rp.BaseMs > 0 {
		base = time.Duration(rp.BaseMs) * time.Millisecond
	}
	if rp != nil && rp.Backoff == "fixed" {
		return base
	}
	shift := attempt - 1
	if shift > maxBackoffShift {
		shift = maxBackoffShift
	}
	return base * time.Duration(int64(1)<<uint(shift))
}
