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

func TestHeartbeatSuccess(t *testing.T) {
	const nodeID = "node-heartbeat"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, _, err = conn.Read(ctx)
		if err != nil {
			t.Errorf("read hello: %v", err)
			return
		}

		welcome, _ := json.Marshal(map[string]any{
			"protocol":  1,
			"id":        "welcome-1",
			"type":      "node.welcome",
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"payload": map[string]any{
				"nodeId":                   nodeID,
				"heartbeatIntervalSeconds": 30,
			},
		})
		if err := conn.Write(ctx, websocket.MessageText, welcome); err != nil {
			t.Errorf("write welcome: %v", err)
			return
		}

		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Errorf("read heartbeat: %v", err)
			return
		}

		var heartbeat Envelope
		if err := json.Unmarshal(data, &heartbeat); err != nil {
			t.Errorf("decode heartbeat: %v", err)
			return
		}

		if heartbeat.Type != "node.heartbeat" {
			t.Errorf("unexpected message: %s", heartbeat.Type)
			return
		}

		var payload Heartbeat
		if err := json.Unmarshal(heartbeat.Payload, &payload); err != nil {
			t.Errorf("decode payload: %v", err)
			return
		}

		if payload.UptimeSeconds != 42 {
			t.Errorf("unexpected uptime: %d", payload.UptimeSeconds)
		}

		ack, _ := json.Marshal(map[string]any{
			"protocol":  1,
			"id":        "ack-1",
			"type":      "node.heartbeat.ack",
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"payload":   map[string]any{},
		})

		if err := conn.Write(ctx, websocket.MessageText, ack); err != nil {
			t.Errorf("write ack: %v", err)
		}
	}))
	defer server.Close()

	cfg := config.Config{
		PanelURL:  server.URL,
		NodeID:    nodeID,
		NodeToken: "test-token",
	}

	client := NewClient(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	err = session.SendHeartbeat(ctx, Heartbeat{
		UptimeSeconds: 42,
		Docker: DockerStatus{
			Available: false,
			Version:   nil,
		},
	})
	if err != nil {
		t.Fatalf("SendHeartbeat: %v", err)
	}
}

func TestHeartbeatRejectsNegativeUptime(t *testing.T) {
	err := (&Session{}).SendHeartbeat(
		context.Background(),
		Heartbeat{UptimeSeconds: -1},
	)

	if err == nil || !strings.Contains(err.Error(), "uptimeSeconds") {
		t.Fatalf("expected uptime validation error, got %v", err)
	}
}
