package jobletclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

// serverName must match the CN/SAN on joblet's server certificate.
const serverName = "joblet"

// Node is one joblet server entry in an rnx-config.yml (address + PEM mTLS credentials).
type Node struct {
	Address   string `yaml:"address"`
	IsDefault *bool  `yaml:"isDefault"`
	Cert      string `yaml:"cert"`
	Key       string `yaml:"key"`
	CA        string `yaml:"ca"`
}

// clientConfig is the subset of rnx-config.yml the engine reads.
type clientConfig struct {
	Nodes map[string]*Node `yaml:"nodes"`
}

// configSearchPaths mirrors rnx's client-config lookup order.
var configSearchPaths = []string{
	"./rnx-config.yml",
	"./config/rnx-config.yml",
	filepath.Join(os.Getenv("HOME"), ".rnx", "rnx-config.yml"),
	"/etc/joblet/rnx-config.yml",
	"/opt/joblet/config/rnx-config.yml",
}

// ConnectOptions selects how the engine reaches joblet.
type ConnectOptions struct {
	// Insecure dials Addr in plaintext (development/e2e only).
	Insecure bool
	Addr     string // used in insecure mode
	// mTLS mode:
	ConfigPath string // explicit rnx-config.yml; empty searches standard paths
	NodeName   string // node entry to use; empty means "default"
	Log        *slog.Logger
}

// Connect dials joblet over mTLS from a config node, or plaintext when Insecure (lazy).
func Connect(opts ConnectOptions) (*grpc.ClientConn, error) {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	if opts.Insecure {
		log.Warn("joblet mTLS disabled (insecure mode) - development only", "addr", opts.Addr)
		return grpc.NewClient(opts.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	node, name, path, err := loadNode(opts.ConfigPath, opts.NodeName)
	if err != nil {
		return nil, fmt.Errorf("%w (set JOBLET_INSECURE=1 for local dev without certs)", err)
	}
	tlsCfg, err := tlsConfigForNode(node)
	if err != nil {
		return nil, err
	}
	log.Info("dialing joblet over mTLS", "addr", node.Address, "node", name, "config", path)
	return grpc.NewClient(node.Address, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
}

// loadNode reads rnx-config.yml and returns the named node plus its resolved
// name and the config path; empty nodeName resolves via defaultNodeName.
func loadNode(configPath, nodeName string) (*Node, string, string, error) {
	path := configPath
	if path == "" {
		path = findConfig()
		if path == "" {
			return nil, "", "", fmt.Errorf("rnx-config.yml not found in standard locations")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("read joblet config %s: %w", path, err)
	}
	var cc clientConfig
	if err := yaml.Unmarshal(data, &cc); err != nil {
		return nil, "", "", fmt.Errorf("parse joblet config %s: %w", path, err)
	}
	var defaults []string
	for n, node := range cc.Nodes {
		if node.IsDefault != nil && *node.IsDefault {
			defaults = append(defaults, n)
		}
	}
	if len(defaults) > 1 {
		sort.Strings(defaults)
		return nil, "", "", fmt.Errorf("multiple nodes marked with isDefault: true in %s: %s (only one node can be the default)",
			path, strings.Join(defaults, ", "))
	}
	name := nodeName
	if name == "" {
		name = defaultNodeName(cc.Nodes)
		if name == "" {
			return nil, "", "", fmt.Errorf("no default node in %s: mark one with isDefault: true or set a node name", path)
		}
	}
	node, ok := cc.Nodes[name]
	if !ok {
		return nil, "", "", fmt.Errorf("node %q not found in %s", name, path)
	}
	if node.Address == "" {
		return nil, "", "", fmt.Errorf("node %q in %s has no address", name, path)
	}
	return node, name, path, nil
}

// tlsConfigForNode builds an mTLS client config from a node's PEM credentials.
func tlsConfigForNode(n *Node) (*tls.Config, error) {
	if n.Cert == "" || n.Key == "" || n.CA == "" {
		return nil, fmt.Errorf("node is missing cert, key, or ca")
	}
	cert, err := tls.X509KeyPair([]byte(n.Cert), []byte(n.Key))
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(n.CA)) {
		return nil, fmt.Errorf("parse CA certificate")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS13,
		ServerName:   serverName,
	}, nil
}

func findConfig() string {
	for _, p := range configSearchPaths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// defaultNodeName mirrors rnx's resolution: the node marked isDefault: true,
// then a node literally named "default" (legacy), then the only node if one.
func defaultNodeName(nodes map[string]*Node) string {
	for name, node := range nodes {
		if node.IsDefault != nil && *node.IsDefault {
			return name
		}
	}
	if _, ok := nodes["default"]; ok {
		return "default"
	}
	if len(nodes) == 1 {
		for name := range nodes {
			return name
		}
	}
	return ""
}
