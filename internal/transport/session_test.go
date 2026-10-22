package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Tyravex/agent/internal/config"
)

const sessionTestNodeID = "session-test-node"

func sessionTestWelcome(t *testing.T, ctx context.Context, conn *websocket.Conn) {
	t.Helper()

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

	if hello.Type != "node.hello" {
		t.Errorf("expected node.hello, got %s", hello.Type)
		return
	}

	err = writeEnvelope(ctx, conn, "node.welcome", Welcome{
		NodeID:                   sessionTestNodeID,
		HeartbeatIntervalSeconds: 30,
	})
	if err != nil {
		t.Errorf("send welcome: %v", err)
	}
}

func sessionTestClient(server *httptest.Server) *Client {
	return NewClient(config.Config{
		PanelURL:  server.URL,
		NodeID:    sessionTestNodeID,
		NodeToken: "test-token",
	})
}

func TestSessionCloseIsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		sessionTestWelcome(t, ctx, conn)

		// Wait for the client to close the connection.
		_, _, _ = conn.Read(ctx)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := sessionTestClient(server).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.Close()
		}()
	}

	wg.Wait()

	select {
	case <-session.Done():
	default:
		t.Fatal("session Done channel should be closed")
	}

	if session.Err() == nil {
		t.Fatal("expected session close reason")
	}
}

func TestSessionDetectsServerDisconnect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		sessionTestWelcome(t, ctx, conn)

		_ = conn.Close(websocket.StatusNormalClosure, "test disconnect")
	}))
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
		t.Fatal("session did not detect server disconnect")
	}

	if session.Err() == nil {
		t.Fatal("expected disconnect error")
	}

	err = session.SendHeartbeat(ctx, Heartbeat{
		UptimeSeconds: 10,
	})

	if err == nil {
		t.Fatal("heartbeat should fail after disconnect")
	}
}

func TestSessionHeartbeatTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		sessionTestWelcome(t, ctx, conn)

		// Receive the heartbeat, but intentionally do not send an ACK.
		_, _, _ = conn.Read(ctx)

		// Wait for the Agent to close the session.
		_, _, _ = conn.Read(ctx)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := sessionTestClient(server).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	heartbeatCtx, heartbeatCancel := context.WithTimeout(
		ctx,
		150*time.Millisecond,
	)
	defer heartbeatCancel()

	err = session.SendHeartbeat(heartbeatCtx, Heartbeat{
		UptimeSeconds: 20,
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected heartbeat deadline exceeded, got %v", err)
	}

	select {
	case <-session.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("session should close after heartbeat timeout")
	}
}

func TestSessionRejectsConcurrentHeartbeat(t *testing.T) {
	received := make(chan struct{}, 1)
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		received <- struct{}{}

		select {
		case <-release:
		case <-ctx.Done():
			return
		}

		_ = writeEnvelope(ctx, conn, "node.heartbeat.ack", struct{}{})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := sessionTestClient(server).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	firstResult := make(chan error, 1)

	go func() {
		firstResult <- session.SendHeartbeat(ctx, Heartbeat{
			UptimeSeconds: 1,
		})
	}()

	select {
	case <-received:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("server did not receive first heartbeat")
	}

	secondErr := session.SendHeartbeat(ctx, Heartbeat{
		UptimeSeconds: 2,
	})

	if secondErr == nil ||
		!strings.Contains(secondErr.Error(), "already pending") {
		close(release)
		t.Fatalf("expected concurrent heartbeat rejection, got %v", secondErr)
	}

	close(release)

	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatalf("first heartbeat failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first heartbeat did not finish")
	}
}

func TestSessionReconnectCreatesNewSession(t *testing.T) {
	var connections atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		connections.Add(1)

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		sessionTestWelcome(t, ctx, conn)

		_, _, _ = conn.Read(ctx)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sessionTestClient(server)

	first, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("first Connect: %v", err)
	}

	first.Close()

	second, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("second Connect: %v", err)
	}
	defer second.Close()

	if first == second {
		t.Fatal("reconnect should create a different Session")
	}

	if got := connections.Load(); got != 2 {
		t.Fatalf("expected 2 connections, got %d", got)
	}

	select {
	case <-second.Done():
		t.Fatal("second session closed unexpectedly")
	default:
	}
}
