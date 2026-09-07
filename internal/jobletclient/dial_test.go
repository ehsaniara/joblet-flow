package jobletclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// selfSignedPEM returns a matching cert/key PEM pair for use as both the client
// credential and its own CA in tests.
func selfSignedPEM(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: serverName},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Unix(1<<31, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM
}

func writeConfig(t *testing.T, node string, cert, key, ca, addr string) string {
	t.Helper()
	// Indent the PEM blocks under YAML block scalars.
	indent := func(s string) string {
		out := ""
		for _, line := range splitLines(s) {
			out += "      " + line + "\n"
		}
		return out
	}
	yml := "nodes:\n  " + node + ":\n    address: \"" + addr + "\"\n" +
		"    cert: |\n" + indent(cert) +
		"    key: |\n" + indent(key) +
		"    ca: |\n" + indent(ca)
	path := filepath.Join(t.TempDir(), "rnx-config.yml")
	if err := os.WriteFile(path, []byte(yml), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func splitLines(s string) []string {
	var lines []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func TestLoadNode_AndTLSConfig(t *testing.T) {
	cert, key := selfSignedPEM(t)
	path := writeConfig(t, "default", cert, key, cert, "10.0.0.1:50051")

	node, gotName, gotPath, err := loadNode(path, "")
	if err != nil {
		t.Fatalf("loadNode: %v", err)
	}
	if node.Address != "10.0.0.1:50051" {
		t.Fatalf("address = %q, want 10.0.0.1:50051", node.Address)
	}
	if gotName != "default" {
		t.Fatalf("name = %q, want default", gotName)
	}
	if gotPath != path {
		t.Fatalf("path = %q, want %q", gotPath, path)
	}

	tlsCfg, err := tlsConfigForNode(node)
	if err != nil {
		t.Fatalf("tlsConfigForNode: %v", err)
	}
	if tlsCfg.ServerName != serverName {
		t.Fatalf("ServerName = %q, want %q", tlsCfg.ServerName, serverName)
	}
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("MinVersion = %d, want TLS 1.3", tlsCfg.MinVersion)
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Fatalf("got %d certificates, want 1", len(tlsCfg.Certificates))
	}
	if tlsCfg.RootCAs == nil {
		t.Fatalf("RootCAs not set")
	}
}

func TestLoadNode_NodeNotFound(t *testing.T) {
	cert, key := selfSignedPEM(t)
	path := writeConfig(t, "default", cert, key, cert, "10.0.0.1:50051")

	if _, _, _, err := loadNode(path, "missing"); err == nil {
		t.Fatalf("expected error for missing node")
	}
}

func TestLoadNode_MultipleDefaultsRejected(t *testing.T) {
	yml := `nodes:
  a:
    address: "10.0.0.1:50051"
    isDefault: true
  b:
    address: "10.0.0.2:50051"
    isDefault: true
`
	path := filepath.Join(t.TempDir(), "rnx-config.yml")
	if err := os.WriteFile(path, []byte(yml), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, _, _, err := loadNode(path, "")
	if err == nil {
		t.Fatalf("expected error for multiple isDefault nodes")
	}
	if !strings.Contains(err.Error(), "isDefault") {
		t.Fatalf("error should mention isDefault, got: %v", err)
	}
}

func TestLoadNode_IsDefaultResolution(t *testing.T) {
	cert, key := selfSignedPEM(t)
	// A node not named "default", marked isDefault: true.
	path := writeConfig(t, "admin", cert, key, cert, "10.0.0.2:50051")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	yml := "nodes:\n  admin:\n    isDefault: true\n" + string(data[len("nodes:\n  admin:\n"):])
	if err := os.WriteFile(path, []byte(yml), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	node, name, _, err := loadNode(path, "")
	if err != nil {
		t.Fatalf("loadNode: %v", err)
	}
	if name != "admin" {
		t.Fatalf("name = %q, want admin", name)
	}
	if node.Address != "10.0.0.2:50051" {
		t.Fatalf("address = %q, want 10.0.0.2:50051", node.Address)
	}
}

func TestTLSConfigForNode_MissingCreds(t *testing.T) {
	if _, err := tlsConfigForNode(&Node{Address: "x:1"}); err == nil {
		t.Fatalf("expected error when cert/key/ca are absent")
	}
}

func TestConnect_InsecureSkipsConfig(t *testing.T) {
	// Insecure mode must not require an rnx-config.yml.
	conn, err := Connect(ConnectOptions{Insecure: true, Addr: "127.0.0.1:50051"})
	if err != nil {
		t.Fatalf("Connect insecure: %v", err)
	}
	defer conn.Close()
}

func TestConnect_MTLSFromConfig(t *testing.T) {
	cert, key := selfSignedPEM(t)
	path := writeConfig(t, "default", cert, key, cert, "127.0.0.1:50051")

	conn, err := Connect(ConnectOptions{ConfigPath: path})
	if err != nil {
		t.Fatalf("Connect mTLS: %v", err)
	}
	defer conn.Close()
}
