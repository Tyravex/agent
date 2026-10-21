package heartbeat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tyravex/agent/internal/config"
)

func TestHeartbeatSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("unexpected method: %s", r.Method)
			}

			if r.URL.Path != "/nodes/node-123/heartbeat" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			if r.Header.Get("x-node-token") != "test-token" {
				t.Error("missing node authentication")
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true}`))
		},
	))
	defer server.Close()

	client := NewClient(config.Config{
		PanelURL:          server.URL,
		NodeID:            "node-123",
		NodeToken:         "test-token",
		HeartbeatInterval: 30 * time.Second,
	})

	if err := client.Send(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHeartbeatUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	))
	defer server.Close()

	client := NewClient(config.Config{
		PanelURL:  server.URL,
		NodeID:    "node-123",
		NodeToken: "invalid-token",
	})

	if err := client.Send(context.Background()); err == nil {
		t.Fatal("expected authentication error")
	}
}

func TestHeartbeatRejectsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":false}`))
		},
	))
	defer server.Close()

	client := NewClient(config.Config{
		PanelURL:  server.URL,
		NodeID:    "node-123",
		NodeToken: "test-token",
	})

	if err := client.Send(context.Background()); err == nil {
		t.Fatal("expected invalid response error")
	}
}
