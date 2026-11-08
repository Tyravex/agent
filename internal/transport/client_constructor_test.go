package transport

import (
	"net/http"
	"testing"

	"github.com/Tyravex/agent/internal/config"
)

func TestNewClientWithHTTPClient(t *testing.T) {
	cfg := config.Config{
		PanelURL:  "https://panel.example",
		NodeID:    "node-1",
		NodeToken: "test-token",
	}

	httpClient := &http.Client{}

	client := NewClientWithHTTPClient(cfg, httpClient)

	if client == nil {
		t.Fatal("constructor returned nil")
	}
	if client.httpClient != httpClient {
		t.Fatal("constructor did not retain supplied HTTP client")
	}
	if client.commandRegistry == nil {
		t.Fatal("constructor did not initialize command registry")
	}
	if client.config.NodeID != cfg.NodeID {
		t.Fatal("constructor changed node identity")
	}
}

func TestNewClientWithNilHTTPClient(t *testing.T) {
	client := NewClientWithHTTPClient(
		config.Config{
			PanelURL: "https://panel.example",
			NodeID:   "node-1",
		},
		nil,
	)

	if client == nil {
		t.Fatal("constructor returned nil")
	}
	if client.httpClient != nil {
		t.Fatal("nil HTTP client should preserve default dialing")
	}
	if client.commandRegistry == nil {
		t.Fatal("constructor did not initialize command registry")
	}
}
