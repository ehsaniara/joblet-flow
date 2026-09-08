// Command joblet-flow runs the flow orchestration engine. See docs/ARCHITECTURE.md.
package main

import (
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "github.com/ehsaniara/joblet-proto/v2/gen/flow"
	"google.golang.org/grpc"

	"github.com/ehsaniara/joblet-flow/internal/config"
	"github.com/ehsaniara/joblet-flow/internal/engine"
	"github.com/ehsaniara/joblet-flow/internal/jobletclient"
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

	eng := engine.New(engine.NewMemStore(), jobletclient.NewRunner(conn, log), log)

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
