package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Tyravex/agent/internal/commands"
	"github.com/Tyravex/agent/internal/config"
	"github.com/coder/websocket"
)

const ownershipIntegrationSessionID = "session-0000000000000001"

func ownershipIntegrationClient(server *httptest.Server) *Client {
	client := NewClient(config.Config{PanelURL: server.URL, NodeID: "node-1", NodeToken: "test-token"})
	client.httpClient = server.Client()
	return client
}

func ownershipIntegrationManager() *commands.OwnershipConnectionManager {
	return commands.NewOwnershipConnectionManager(commands.NewOwnershipSyncManager("node-1"))
}

func ownershipIntegrationSnapshot(sessionID string, revision uint64) map[string]any {
	return map[string]any{
		"type": "ownership.snapshot", "session_id": sessionID, "node_id": "node-1", "revision": revision, "ttl_seconds": 60,
		"assignments": []any{map[string]any{"service_id": "minecraft-1", "node_id": "node-1", "enabled": true}},
	}
}

func ownershipIntegrationServer(t *testing.T, afterWelcome func(context.Context, *websocket.Conn)) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/v1/ws" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		if r.Header.Get("X-Node-Id") != "node-1" || r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		hello, err := readEnvelope(ctx, conn)
		if err != nil {
			return
		}
		if hello.Type != "node.hello" {
			return
		}
		if err := writeEnvelope(ctx, conn, "node.welcome", Welcome{NodeID: "node-1", HeartbeatIntervalSeconds: 30, SessionID: ownershipIntegrationSessionID}); err != nil {
			return
		}
		if afterWelcome != nil {
			afterWelcome(ctx, conn)
		}
	}))
}

func waitOwnershipReady(t *testing.T, ctx context.Context, session *Session, manager *commands.OwnershipConnectionManager) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		status := manager.Status()
		if status.State == commands.OwnershipReady {
			if status.Revision != 1 {
				t.Fatalf("unexpected revision: %d", status.Revision)
			}
			return
		}
		select {
		case <-session.Done():
			t.Fatalf("session closed before READY: %v", session.Err())
		case <-timer.C:
			t.Fatal("ownership did not become READY")
		case <-ctx.Done():
			t.Fatalf("context canceled before READY: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestOwnershipWSSStartsNotReady(t *testing.T) {
	server := ownershipIntegrationServer(t, func(ctx context.Context, conn *websocket.Conn) { _, _, _ = conn.Read(ctx) })
	defer server.Close()
	manager := ownershipIntegrationManager()
	client := ownershipIntegrationClient(server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.Welcome().SessionID != ownershipIntegrationSessionID {
		t.Fatalf("unexpected session ID: %q", session.Welcome().SessionID)
	}
	if got := manager.ActiveGeneration(); got != 1 {
		t.Fatalf("expected generation 1, got %d", got)
	}
	if status := manager.Status(); status.State != commands.OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %+v", status)
	}
	session.Close()
	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf("expected inactive generation after close, got %d", got)
	}
}

func TestOwnershipWSSValidSnapshotBecomesReady(t *testing.T) {
	sendSnapshot := make(chan struct{})
	snapshotSent := make(chan struct{})
	server := ownershipIntegrationServer(t, func(ctx context.Context, conn *websocket.Conn) {
		select {
		case <-sendSnapshot:
		case <-ctx.Done():
			return
		}
		if err := writeEnvelope(ctx, conn, "ownership.snapshot", ownershipIntegrationSnapshot(ownershipIntegrationSessionID, 1)); err != nil {
			return
		}
		close(snapshotSent)
		_, _, _ = conn.Read(ctx)
	})
	defer server.Close()
	manager := ownershipIntegrationManager()
	client := ownershipIntegrationClient(server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	close(sendSnapshot)
	select {
	case <-snapshotSent:
	case <-ctx.Done():
		t.Fatal("server did not send snapshot")
	}
	waitOwnershipReady(t, ctx, session, manager)
	session.Close()
	if got := manager.Status().State; got != commands.OwnershipNotReady {
		t.Fatalf("expected NOT_READY after close, got %s", got)
	}
}

func TestOwnershipWSSWrongSessionClosesConnection(t *testing.T) {
	sendSnapshot := make(chan struct{})
	server := ownershipIntegrationServer(t, func(ctx context.Context, conn *websocket.Conn) {
		select {
		case <-sendSnapshot:
		case <-ctx.Done():
			return
		}
		_ = writeEnvelope(ctx, conn, "ownership.snapshot", ownershipIntegrationSnapshot("wrong-session", 1))
		_, _, _ = conn.Read(ctx)
	})
	defer server.Close()
	manager := ownershipIntegrationManager()
	client := ownershipIntegrationClient(server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	close(sendSnapshot)
	select {
	case <-session.Done():
	case <-ctx.Done():
		t.Fatal("wrong session did not close WebSocket")
	}
	if !errors.Is(session.Err(), commands.ErrOwnershipSessionMismatch) {
		t.Fatalf("expected session mismatch, got %v", session.Err())
	}
	if got := manager.Status().State; got != commands.OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}
	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf("expected inactive generation, got %d", got)
	}
}

func TestOwnershipWSSDisconnectRevokesAuthorization(t *testing.T) {
	sendSnapshot := make(chan struct{})
	closeConnection := make(chan struct{})
	server := ownershipIntegrationServer(t, func(ctx context.Context, conn *websocket.Conn) {
		select {
		case <-sendSnapshot:
		case <-ctx.Done():
			return
		}
		if err := writeEnvelope(ctx, conn, "ownership.snapshot", ownershipIntegrationSnapshot(ownershipIntegrationSessionID, 1)); err != nil {
			return
		}
		select {
		case <-closeConnection:
		case <-ctx.Done():
		}
	})
	defer server.Close()
	manager := ownershipIntegrationManager()
	client := ownershipIntegrationClient(server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	close(sendSnapshot)
	waitOwnershipReady(t, ctx, session, manager)
	close(closeConnection)
	select {
	case <-session.Done():
	case <-ctx.Done():
		t.Fatal("disconnect did not stop session")
	}
	if got := manager.Status().State; got != commands.OwnershipNotReady {
		t.Fatalf("expected NOT_READY after disconnect, got %s", got)
	}
	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf("expected inactive generation after disconnect, got %d", got)
	}
}

func TestOwnershipWSSActivationCloseRace(t *testing.T) {
	const iterations = 50
	for i := 0; i < iterations; i++ {
		t.Run(fmt.Sprintf("iteration-%d", i), func(t *testing.T) {
			manager := ownershipIntegrationManager()
			ownership, err := newOwnershipSession(context.Background(), manager)
			if err != nil {
				t.Fatal(err)
			}
			session := &Session{
				welcome: Welcome{
					NodeID:    "node-1",
					SessionID: ownershipIntegrationSessionID,
				},
				ownership: ownership,
			}
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				_ = session.ActivateOwnership(
					context.Background(),
					ownershipIntegrationSessionID,
				)
			}()
			go func() {
				defer wg.Done()
				session.mu.Lock()
				session.closed = true
				if session.ownership != nil {
					session.ownership.close()
				}
				session.mu.Unlock()
			}()
			wg.Wait()
			if got := manager.ActiveGeneration(); got != 0 {
				t.Fatalf("closed session retained generation %d", got)
			}
			if got := manager.Status().State; got != commands.OwnershipNotReady {
				t.Fatalf("closed session retained authorization: %s", got)
			}
		})
	}
}
