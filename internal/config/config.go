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
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		ListenAddr:       env("FLOW_LISTEN_ADDR", ":50055"),
		JobletInsecure:   os.Getenv("JOBLET_INSECURE") == "1",
		JobletAddr:       env("JOBLET_ADDR", "localhost:50051"),
		JobletConfigPath: os.Getenv("JOBLET_CONFIG"),
		JobletNode:       os.Getenv("JOBLET_NODE"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
