// Command flow-store is the flow durable-state sink subprocess. It listens on a
// Unix socket for length-prefixed StoreEvents from flow-core and applies each
// to a storage backend. Third-party-backed backends live here, never in
// flow-core; this build ships only the stdlib local-disk backend.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/ehsaniara/joblet-flow/internal/store"
	storepb "github.com/ehsaniara/joblet-flow/internal/store/proto/gen"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	socket := env("FLOW_STORE_SOCKET", "/opt/joblet-flow/run/flow-store.sock")
	dir := env("FLOW_STORE_DIR", "/opt/joblet-flow/state")

	backend, err := store.NewLocalBackend(dir)
	if err != nil {
		log.Error("failed to open backend", "error", err)
		os.Exit(1)
	}
	defer backend.Close()

	// Unix socket paths are capped (~108 bytes on Linux); fail clearly rather
	// than with a cryptic "bind: invalid argument".
	if len(socket) > 100 {
		log.Error("socket path too long for a unix socket", "len", len(socket), "max", 100, "socket", socket)
		os.Exit(1)
	}

	// A stale socket from a prior run would block Listen.
	_ = os.Remove(socket)
	if err := os.MkdirAll(dirOf(socket), 0o700); err != nil {
		log.Error("failed to create socket dir", "error", err)
		os.Exit(1)
	}
	lis, err := net.Listen("unix", socket)
	if err != nil {
		log.Error("failed to listen", "socket", socket, "error", err)
		os.Exit(1)
	}
	defer lis.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		lis.Close()
	}()

	apply := func(ev *storepb.StoreEvent) error { return backend.Apply(ctx, ev) }

	log.Info("flow-store listening", "socket", socket, "dir", dir)
	for {
		conn, err := lis.Accept()
		if err != nil {
			if ctx.Err() != nil {
				log.Info("shutting down")
				return
			}
			log.Warn("accept failed", "error", err)
			continue
		}
		// One flow-core connection at a time; serve it to completion.
		if err := store.ServeConn(conn, backend, apply); err != nil {
			log.Warn("connection ended", "error", err)
		}
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
