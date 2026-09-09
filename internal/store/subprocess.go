package store

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// Subprocess supervises the flow-store child: it starts the binary and restarts
// it with capped exponential backoff if it exits, until the context is
// cancelled. It mirrors joblet's persist supervisor.
type Subprocess struct {
	bin    string
	socket string
	dir    string
	log    *slog.Logger

	minDelay time.Duration
	maxDelay time.Duration
}

// NewSubprocess supervises bin, passing it the socket and state dir via env.
func NewSubprocess(bin, socket, dir string, log *slog.Logger) *Subprocess {
	if log == nil {
		log = slog.Default()
	}
	return &Subprocess{
		bin: bin, socket: socket, dir: dir, log: log,
		minDelay: time.Second, maxDelay: 30 * time.Second,
	}
}

// Run supervises the child until ctx is cancelled.
func (s *Subprocess) Run(ctx context.Context) {
	delay := s.minDelay
	for {
		if ctx.Err() != nil {
			return
		}
		cmd := exec.CommandContext(ctx, s.bin)
		cmd.Env = append(os.Environ(),
			"FLOW_STORE_SOCKET="+s.socket,
			"FLOW_STORE_DIR="+s.dir,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			s.log.Error("failed to start flow-store", "error", err)
		} else {
			s.log.Info("flow-store started", "pid", cmd.Process.Pid, "socket", s.socket)
			err := cmd.Wait()
			if ctx.Err() != nil {
				return // shutdown, not a crash
			}
			s.log.Warn("flow-store exited, restarting", "error", err, "delay", delay)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			delay *= 2
			if delay > s.maxDelay {
				delay = s.maxDelay
			}
		}
	}
}
