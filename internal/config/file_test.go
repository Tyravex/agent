package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	content := `
panel:
  url: "http://localhost:3000"
node:
  id: "test-node"
  token: "test-token"
heartbeat:
  interval: "30s"
docker:
  enabled: true
  host: "npipe:////./pipe/docker_engine"
api:
  listen: "127.0.0.1:8080"
  token: "1234567890abcdef1234567890abcdef"
`

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.NodeID != "test-node" {
		t.Fatalf("unexpected node ID: %s", cfg.NodeID)
	}

	if cfg.HeartbeatInterval != 30*time.Second {
		t.Fatalf("unexpected interval: %v", cfg.HeartbeatInterval)
	}

	if !cfg.DockerEnabled {
		t.Fatal("Docker should be enabled")
	}

	if cfg.APIListen != "127.0.0.1:8080" {
		t.Fatalf("unexpected API address: %s", cfg.APIListen)
	}

	if cfg.APIToken != "1234567890abcdef1234567890abcdef" {
		t.Fatal("unexpected API token")
	}
}

func TestLoadFileMissing(t *testing.T) {
	_, err := LoadFile("nonexistent-config.yml")
	if err == nil {
		t.Fatal("expected missing configuration error")
	}
}
