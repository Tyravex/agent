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
)

func TestSessionRejectsInstallCommandWithoutExecution(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(map[string]any)
		wantError string
	}{
		{
			name:      "valid installation not yet supported",
			edit:      func(map[string]any) {},
			wantError: "not supported yet",
		},
		{
			name: "invalid installation command",
			edit: func(payload map[string]any) {
				payload["operation"] = "server.start"
			},
			wantError: "invalid service installation command",
		},
		{
			name: "wrong target node",
			edit: func(payload map[string]any) {
				parameters := payload["parameters"].(map[string]any)
				parameters["nodeId"] = "another-node"
			},
			wantError: "different node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					conn, err := websocket.Accept(w, r, nil)
					if err != nil {
						return
					}
					defer conn.CloseNow()

					ctx, cancel := context.WithTimeout(
						r.Context(),
						5*time.Second,
					)
					defer cancel()

					sessionTestWelcome(t, ctx, conn)

					var payload map[string]any
					if err := json.Unmarshal(
						validServiceInstallCommandJSON(t),
						&payload,
					); err != nil {
						t.Error(err)
						return
					}

					tt.edit(payload)
					// Use the authenticated test session's node ID unless
					// this test intentionally targets a different node.
					if tt.name != "wrong target node" {
						parameters := payload["parameters"].(map[string]any)
						parameters["nodeId"] = sessionTestNodeID
					}

					if err := writeEnvelope(
						ctx,
						conn,
						"command.execute",
						payload,
					); err != nil {
						t.Error(err)
						return
					}

					// The Agent should close the connection.
					// No command.accepted or command.result is expected.
					_, _, _ = conn.Read(ctx)
				},
			))
			defer server.Close()

			ctx, cancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			defer cancel()

			session, err := sessionTestClient(server).Connect(ctx)
			if err != nil {
				t.Fatalf("Connect: %v", err)
			}
			defer session.Close()

			select {
			case <-session.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("session did not reject installation command")
			}

			if err := session.Err(); err == nil ||
				!strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf(
					"expected error containing %q, got %v",
					tt.wantError,
					err,
				)
			}
		})
	}
}

func TestSessionHeartbeatStillWorks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()

			ctx, cancel := context.WithTimeout(
				r.Context(),
				5*time.Second,
			)
			defer cancel()

			sessionTestWelcome(t, ctx, conn)

			envelope, err := readEnvelope(ctx, conn)
			if err != nil {
				t.Error(err)
				return
			}

			if envelope.Type != "node.heartbeat" {
				t.Errorf(
					"expected node.heartbeat, got %s",
					envelope.Type,
				)
				return
			}

			if err := writeEnvelope(
				ctx,
				conn,
				"node.heartbeat.ack",
				struct{}{},
			); err != nil {
				t.Error(err)
				return
			}

			// Keep connection open until the Agent closes it.
			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	session, err := sessionTestClient(server).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	if err := session.SendHeartbeat(ctx, Heartbeat{
		UptimeSeconds: 10,
	}); err != nil {
		t.Fatalf("heartbeat should succeed: %v", err)
	}

	select {
	case <-session.Done():
		t.Fatalf("session closed unexpectedly: %v", session.Err())
	default:
	}
}
