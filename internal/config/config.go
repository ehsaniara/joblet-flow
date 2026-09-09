// Package config loads engine settings from the environment.
package config

import "os"

// Config holds the engine's runtime settings.
type Config struct {
	// ListenAddr is where the FlowService gRPC server listens.
	ListenAddr string

	// Joblet connection; mTLS by default, or plaintext when JobletInsecure. See docs/CONFIGURATION.md.
	JobletInsecure   bool
	JobletAddr       string // insecure-mode target
	JobletConfigPath string // explicit rnx-config.yml; empty searches standard paths
	JobletNode       string // node entry to use; empty resolves the isDefault: true node

	// Durable state sink (flow-store subprocess). Empty StoreSocket keeps state
	// in memory only, with no subprocess.
	StoreSocket string // Unix socket flow-store listens on; empty disables it
	StoreDir    string // flow-store's on-disk state directory
	StoreBin    string // flow-store binary path; empty resolves next to the engine
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		ListenAddr:       env("FLOW_LISTEN_ADDR", ":50055"),
		JobletInsecure:   os.Getenv("JOBLET_INSECURE") == "1",
		JobletAddr:       env("JOBLET_ADDR", "localhost:50051"),
		JobletConfigPath: os.Getenv("JOBLET_CONFIG"),
		JobletNode:       os.Getenv("JOBLET_NODE"),
		StoreSocket:      os.Getenv("FLOW_STORE_SOCKET"),
		StoreDir:         env("FLOW_STORE_DIR", "/opt/joblet-flow/state"),
		StoreBin:         os.Getenv("FLOW_STORE_BIN"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
