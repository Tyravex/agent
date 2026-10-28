package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tyravex/agent/internal/commands"
	"github.com/Tyravex/agent/internal/config"
	"github.com/coder/websocket"
)

func TestOwnershipLimitsRawEnvelopeSize(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if _, _, err = conn.Read(ctx); err != nil {
			return
		}
		if err = writeEnvelope(ctx, conn, "node.welcome", Welcome{NodeID: "node-1", HeartbeatIntervalSeconds: 30, SessionID: ownershipIntegrationSessionID}); err != nil {
			return
		}
		_ = writeEnvelope(ctx, conn, "unknown.message", map[string]any{"padding": strings.Repeat("a", 20*1024)})
		_, _, _ = conn.Read(ctx)
	}))
	defer server.Close()
	client := NewClient(config.Config{PanelURL: server.URL, NodeID: "node-1", NodeToken: "test-token"})
	client.httpClient = server.Client()
	manager := commands.NewOwnershipConnectionManager(commands.NewOwnershipSyncManager("node-1"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	select {
	case <-session.Done():
	case <-ctx.Done():
		t.Fatal("oversized ordinary message was not rejected")
	}
	if session.Err() == nil || !strings.Contains(session.Err().Error(), "exceeds size limit") {
		t.Fatalf("unexpected error: %v", session.Err())
	}
	if manager.Status().State != commands.OwnershipNotReady {
		t.Fatal("ownership must remain NOT_READY")
	}
	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf("expected inactive generation, got %d", got)
	}
}

func TestOwnershipLimitsRejectsOversizedSnapshot(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if _, _, err = conn.Read(ctx); err != nil {
			return
		}
		if err = writeEnvelope(ctx, conn, "node.welcome", Welcome{NodeID: "node-1", HeartbeatIntervalSeconds: 30, SessionID: ownershipIntegrationSessionID}); err != nil {
			return
		}
		_ = writeEnvelope(ctx, conn, commands.OwnershipSnapshotMessageType, map[string]any{
			"type": "ownership.snapshot", "session_id": ownershipIntegrationSessionID, "node_id": "node-1", "revision": 1, "ttl_seconds": 60,
			"assignments": []any{}, "padding": strings.Repeat("a", maxOwnershipEnvelopeBytes),
		})
		_, _, _ = conn.Read(ctx)
	}))
	defer server.Close()
	client := NewClient(config.Config{PanelURL: server.URL, NodeID: "node-1", NodeToken: "test-token"})
	client.httpClient = server.Client()
	manager := commands.NewOwnershipConnectionManager(commands.NewOwnershipSyncManager("node-1"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	select {
	case <-session.Done():
	case <-ctx.Done():
		t.Fatal("oversized snapshot did not close connection")
	}
	if manager.Status().State != commands.OwnershipNotReady {
		t.Fatal("oversized snapshot retained authorization")
	}
	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf("oversized snapshot retained generation %d", got)
	}
}

func TestOwnershipLimitsClosedSessionCannotActivate(t *testing.T) {
	manager := commands.NewOwnershipConnectionManager(commands.NewOwnershipSyncManager("node-1"))
	ownership, err := newOwnershipSession(context.Background(), manager)
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{ownership: ownership, closed: true, readErr: errSessionClosed}
	err = session.ActivateOwnership(context.Background(), "session-1")
	if !errors.Is(err, errSessionClosed) {
		t.Fatalf("expected closed session, got %v", err)
	}
	if manager.ActiveGeneration() != 0 {
		t.Fatal("closed session activated ownership")
	}
	ownership.close()
}
