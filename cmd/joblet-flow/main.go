// Command joblet-flow runs the flow orchestration engine. See docs/ARCHITECTURE.md.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	pb "github.com/ehsaniara/joblet-proto/v2/gen/flow"
	"google.golang.org/grpc"

	"github.com/ehsaniara/joblet-flow/internal/config"
	"github.com/ehsaniara/joblet-flow/internal/engine"
	"github.com/ehsaniara/joblet-flow/internal/jobletclient"
	"github.com/ehsaniara/joblet-flow/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	conn, err := jobletclient.Connect(jobletclient.ConnectOptions{
		Insecure:   cfg.JobletInsecure,
		Addr:       cfg.JobletAddr,
		ConfigPath: cfg.JobletConfigPath,
		NodeName:   cfg.JobletNode,
		Log:        log,
	})
	if err != nil {
		log.Error("failed to connect to joblet", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	// State is authoritative in memory; when a flow-store socket is configured,
	// mutations are also published and shipped to the supervised flow-store
	// subprocess for durability.
	storeCtx, stopStore := context.WithCancel(context.Background())
	defer stopStore()
	var st engine.Store = engine.NewMemStore()
	if cfg.StoreSocket != "" {
		ps := store.NewPubSub(0)
		defer ps.Close()
		st = engine.NewPublishingStore(st, ps)

		bin := cfg.StoreBin
		if bin == "" {
			bin = resolveFlowStoreBin(log)
		}
		go store.NewSubprocess(bin, cfg.StoreSocket, cfg.StoreDir, log).Run(storeCtx)

		shipper, unsub := store.NewShipper(cfg.StoreSocket, ps, log)
		defer unsub()
		go shipper.Run(storeCtx)
		log.Info("durable state enabled", "flow_store_socket", cfg.StoreSocket, "dir", cfg.StoreDir)
	}

	eng := engine.New(st, jobletclient.NewRunner(conn, log), log)

	srv := grpc.NewServer()
	pb.RegisterFlowServiceServer(srv, eng)

	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		log.Error("failed to listen", "addr", cfg.ListenAddr, "error", err)
		os.Exit(1)
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info("shutting down")
		srv.GracefulStop()
	}()

	// The joblet target is logged by jobletclient.Connect with its real
	// resolved address (config node in mTLS mode, JOBLET_ADDR when insecure).
	log.Info("joblet-flow engine listening", "addr", cfg.ListenAddr)
	if err := srv.Serve(lis); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// resolveFlowStoreBin finds the flow-store binary next to the running engine.
func resolveFlowStoreBin(log *slog.Logger) string {
	exe, err := os.Executable()
	if err != nil {
		log.Warn("cannot resolve executable path; assuming flow-store on PATH", "error", err)
		return "flow-store"
	}
	return filepath.Join(filepath.Dir(exe), "flow-store")
}
