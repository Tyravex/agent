package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Tyravex/agent/internal/commands"
	"github.com/Tyravex/agent/internal/config"
)

func TestOwnershipWebSocketRejectsPlainHTTP(t *testing.T) {
	manager := commands.NewOwnershipConnectionManager(
		commands.NewOwnershipSyncManager("node-1"),
	)

	client := NewClient(config.Config{
		PanelURL:  "http://localhost:3000",
		NodeID:    "node-1",
		NodeToken: "test-token",
	})

	_, err := client.ConnectWithOwnership(
		context.Background(),
		manager,
	)

	if !errors.Is(err, ErrOwnershipRequiresTLS) {
		t.Fatalf("expected TLS requirement, got %v", err)
	}
}

func TestOwnershipWebSocketRejectsMissingManager(t *testing.T) {
	client := NewClient(config.Config{
		PanelURL:  "https://panel.example.com",
		NodeID:    "node-1",
		NodeToken: "test-token",
	})

	_, err := client.ConnectWithOwnership(
		context.Background(),
		nil,
	)

	if !errors.Is(err, ErrOwnershipTransportDisabled) {
		t.Fatalf("expected disabled ownership, got %v", err)
	}
}

func TestOwnershipWebSocketRejectsSnapshotOnDefaultConnection(
	t *testing.T,
) {
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

			if err := writeEnvelope(
				ctx,
				conn,
				"node.welcome",
				Welcome{
					NodeID:                   "node-1",
					HeartbeatIntervalSeconds: 30,
				},
			); err != nil {
				return
			}

			_ = writeEnvelope(
				ctx,
				conn,
				"ownership.snapshot",
				map[string]any{
					"type":        "ownership.snapshot",
					"session_id":  "session-1",
					"node_id":     "node-1",
					"revision":    1,
					"ttl_seconds": 60,
					"assignments": []any{},
				},
			)

			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	client := NewClient(config.Config{
		PanelURL:  server.URL,
		NodeID:    "node-1",
		NodeToken: "test-token",
	})

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	session, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	select {
	case <-session.Done():
	case <-ctx.Done():
		t.Fatal("default connection did not reject ownership")
	}

	if !errors.Is(
		session.Err(),
		ErrOwnershipTransportDisabled,
	) {
		t.Fatalf(
			"expected disabled ownership, got %v",
			session.Err(),
		)
	}
}

func TestOwnershipWebSocketActivationRequiresConfiguredSession(
	t *testing.T,
) {
	var session Session

	session.done = make(chan struct{})

	err := session.ActivateOwnership(
		context.Background(),
		"session-1",
	)

	if !errors.Is(err, ErrOwnershipSessionUnavailable) {
		t.Fatalf(
			"expected unavailable ownership session, got %v",
			err,
		)
	}
}
