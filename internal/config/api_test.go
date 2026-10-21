package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAPIListen(t *testing.T) {
	valid := map[string]string{
		"":               "127.0.0.1:8080",
		"127.0.0.1:9090": "127.0.0.1:9090",
		"[::1]:8080":     "[::1]:8080",
	}

	for input, expected := range valid {
		actual, err := ValidateAPIListen(input)
		if err != nil {
			t.Errorf("expected valid address %q: %v", input, err)
			continue
		}
		if actual != expected {
			t.Errorf("expected %q, got %q", expected, actual)
		}
	}

	invalid := []string{
		"0.0.0.0:8080",
		":8080",
		"localhost:8080",
		"192.168.1.10:8080",
		"127.0.0.1:0",
		"127.0.0.1:65536",
		"invalid",
	}

	for _, address := range invalid {
		if _, err := ValidateAPIListen(address); err == nil {
			t.Errorf("expected invalid address: %q", address)
		}
	}
}

func TestLoadFileAPIConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")

	content := `
panel:
  url: "http://localhost:3000"
node:
  id: "test-node"
  token: "heartbeat-secret"
heartbeat:
  interval: "30s"
docker:
  enabled: false
api:
  listen: "127.0.0.1:9090"
  token: "1234567890abcdef1234567890abcdef"
`

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.APIListen != "127.0.0.1:9090" {
		t.Fatalf("unexpected API address: %s", cfg.APIListen)
	}

	if cfg.APIToken != "1234567890abcdef1234567890abcdef" {
		t.Fatal("unexpected API token")
	}
}

func TestRejectSameAPIToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")

	token := "1234567890abcdef1234567890abcdef"

	content := `
panel:
  url: "http://localhost:3000"
node:
  id: "test-node"
  token: "` + token + `"
api:
  listen: "127.0.0.1:8080"
  token: "` + token + `"
`

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected duplicate token rejection")
	}

	if strings.Contains(err.Error(), token) {
		t.Fatal("error must not expose token")
	}
}

func TestRejectMissingAPIToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")

	content := `
panel:
  url: "http://localhost:3000"
node:
  id: "test-node"
  token: "heartbeat-secret"
`

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected missing API token error")
	}
}
