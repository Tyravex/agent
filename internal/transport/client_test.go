package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Tyravex/agent/internal/config"
)

func TestWebsocketURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://localhost:3000", "ws://localhost:3000/agent/v1/ws"},
		{"https://panel.example.com", "wss://panel.example.com/agent/v1/ws"},
		{"https://panel.example.com/api", "wss://panel.example.com/api/agent/v1/ws"},
	}

	for _, tt := range tests {
		got, err := websocketURL(tt.input)
		if err != nil {
			t.Fatalf("websocketURL(%q): %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("websocketURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestClientHandshake(t *testing.T) {
	const nodeID = "node-test"
	const token = "test-node-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/v1/ws" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}

		if r.Header.Get("X-Node-Id") != nodeID ||
			r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Errorf("read hello: %v", err)
			return
		}

		var hello Envelope
		if err := json.Unmarshal(data, &hello); err != nil {
			t.Errorf("decode hello: %v", err)
			return
		}

		if hello.Protocol != 1 || hello.Type != "node.hello" {
			t.Errorf("unexpected hello: %+v", hello)
			return
		}

		welcome := map[string]any{
			"protocol":  1,
			"id":        "welcome-001",
			"type":      "node.welcome",
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"payload": map[string]any{
				"nodeId":                   nodeID,
				"heartbeatIntervalSeconds": 30,
			},
		}

		data, err = json.Marshal(welcome)
		if err != nil {
			t.Errorf("marshal welcome: %v", err)
			return
		}

		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			t.Errorf("write welcome: %v", err)
		}
	}))
	defer server.Close()

	cfg := config.Config{
		PanelURL:  server.URL,
		NodeID:    nodeID,
		NodeToken: token,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := NewClient(cfg).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()
	welcome := session.Welcome()

	if welcome.NodeID != nodeID {
		t.Errorf("welcome nodeId = %q", welcome.NodeID)
	}
	if welcome.HeartbeatIntervalSeconds != 30 {
		t.Errorf("heartbeat interval = %d", welcome.HeartbeatIntervalSeconds)
	}
}

func TestClientRejectsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := config.Config{
		PanelURL:  server.URL,
		NodeID:    "node-test",
		NodeToken: "invalid-token",
	}

	_, err := NewClient(cfg).Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("expected HTTP 401 error, got %v", err)
	}
}
