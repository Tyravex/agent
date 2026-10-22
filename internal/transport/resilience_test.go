package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestSessionRejectsHeartbeatAfterClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()

			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()

			sessionTestWelcome(t, ctx, conn)
			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := sessionTestClient(server).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	session.Close()

	err = session.SendHeartbeat(ctx, Heartbeat{
		UptimeSeconds: 10,
	})

	if !errors.Is(err, errSessionClosed) {
		t.Fatalf("expected closed session error, got %v", err)
	}

	select {
	case <-session.Done():
	default:
		t.Fatal("session should be stopped")
	}
}

func TestSessionRejectsUnexpectedACK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()

			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()

			sessionTestWelcome(t, ctx, conn)

			if err := writeEnvelope(
				ctx,
				conn,
				"node.heartbeat.ack",
				struct{}{},
			); err != nil {
				t.Errorf("send unexpected ACK: %v", err)
				return
			}

			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := sessionTestClient(server).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	select {
	case <-session.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("session did not reject unexpected ACK")
	}

	if err := session.Err(); err == nil ||
		!strings.Contains(err.Error(), "unexpected heartbeat ACK") {
		t.Fatalf("expected unexpected ACK error, got %v", err)
	}
}

func TestSessionRejectsInvalidACKAndCloses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()

			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()

			sessionTestWelcome(t, ctx, conn)

			_, _, err = conn.Read(ctx)
			if err != nil {
				return
			}

			_ = writeEnvelope(
				ctx,
				conn,
				"node.heartbeat.ack",
				map[string]any{"unexpected": true},
			)

			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := sessionTestClient(server).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	err = session.SendHeartbeat(ctx, Heartbeat{
		UptimeSeconds: 10,
	})

	if err == nil || !strings.Contains(err.Error(), "ACK payload") {
		t.Fatalf("expected invalid ACK error, got %v", err)
	}

	select {
	case <-session.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("session did not close after invalid ACK")
	}
}

func TestHandshakeRejectsInvalidWelcome(t *testing.T) {
	tests := []struct {
		name    string
		payload any
	}{
		{
			name: "wrong node ID",
			payload: Welcome{
				NodeID:                   "another-node",
				HeartbeatIntervalSeconds: 30,
			},
		},
		{
			name: "heartbeat interval too small",
			payload: Welcome{
				NodeID:                   sessionTestNodeID,
				HeartbeatIntervalSeconds: 4,
			},
		},
		{
			name: "heartbeat interval too large",
			payload: Welcome{
				NodeID:                   sessionTestNodeID,
				HeartbeatIntervalSeconds: 301,
			},
		},
		{
			name: "unknown payload field",
			payload: map[string]any{
				"nodeId":                   sessionTestNodeID,
				"heartbeatIntervalSeconds": 30,
				"unexpected":               true,
			},
		},
		{
			name:    "null payload",
			payload: nil,
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

					_, _, err = conn.Read(ctx)
					if err != nil {
						return
					}

					_ = writeEnvelope(
						ctx,
						conn,
						"node.welcome",
						tt.payload,
					)

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
			if err == nil {
				session.Close()
				t.Fatal("expected handshake rejection")
			}
		})
	}
}

func TestDecodeStrictRejectsTrailingJSON(t *testing.T) {
	var welcome Welcome

	err := decodeStrict(
		[]byte(`{"nodeId":"node-1","heartbeatIntervalSeconds":30} true`),
		&welcome,
	)
	if err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
}

func TestReadEnvelopeRejectsInvalidPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload json.RawMessage
	}{
		{"null", json.RawMessage(`null`)},
		{"array", json.RawMessage(`[]`)},
		{"string", json.RawMessage(`"hello"`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var object map[string]json.RawMessage

			err := json.Unmarshal(tt.payload, &object)
			if err == nil && object != nil {
				t.Fatal("expected non-object payload")
			}
		})
	}
}
