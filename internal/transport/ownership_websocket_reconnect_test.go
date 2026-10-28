package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Tyravex/agent/internal/commands"
)

func TestOwnershipReconnectOldConnectionCannotRevokeNew(t *testing.T) {
	const (
		firstSessionID  = "session-0000000000000001"
		secondSessionID = "session-0000000000000002"
	)

	var connectionNumber atomic.Uint64

	server := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/agent/v1/ws" {
				http.Error(
					w,
					"wrong websocket path",
					http.StatusNotFound,
				)
				return
			}

			if r.Header.Get("X-Node-Id") != "node-1" ||
				r.Header.Get("Authorization") != "Bearer test-token" {
				http.Error(
					w,
					"unauthorized",
					http.StatusUnauthorized,
				)
				return
			}

			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Errorf("websocket accept: %v", err)
				return
			}
			defer conn.CloseNow()

			ctx, cancel := context.WithTimeout(
				r.Context(),
				10*time.Second,
			)
			defer cancel()

			hello, err := readEnvelope(ctx, conn)
			if err != nil {
				t.Errorf("read node.hello: %v", err)
				return
			}

			if hello.Type != "node.hello" {
				t.Errorf(
					"unexpected handshake message: %q",
					hello.Type,
				)
				return
			}

			number := connectionNumber.Add(1)

			sessionID := fmt.Sprintf(
				"session-%016d",
				number,
			)

			err = writeEnvelope(
				ctx,
				conn,
				"node.welcome",
				Welcome{
					NodeID:                   "node-1",
					HeartbeatIntervalSeconds: 30,
					SessionID:                sessionID,
				},
			)
			if err != nil {
				t.Errorf("write node.welcome: %v", err)
				return
			}

			// Keep this connection alive until the
			// client closes it. Each reconnect receives
			// its own Panel-issued session identifier.
			for {
				if _, _, err := conn.Read(ctx); err != nil {
					return
				}
			}
		}),
	)
	defer server.Close()

	client := ownershipIntegrationClient(server)
	manager := ownershipIntegrationManager()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	first, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatalf("connect first ownership session: %v", err)
	}
	defer first.Close()

	if got := first.Welcome().SessionID; got != firstSessionID {
		t.Fatalf(
			"first session ID = %q, want %q",
			got,
			firstSessionID,
		)
	}

	if got := manager.ActiveGeneration(); got != 1 {
		t.Fatalf(
			"first generation = %d, want 1",
			got,
		)
	}

	if status := manager.Status(); status.State != commands.OwnershipNotReady {
		t.Fatalf(
			"first connection must start NOT_READY: %+v",
			status,
		)
	}

	second, err := client.ConnectWithOwnership(ctx, manager)
	if err != nil {
		t.Fatalf("connect second ownership session: %v", err)
	}
	defer second.Close()

	if got := second.Welcome().SessionID; got != secondSessionID {
		t.Fatalf(
			"second session ID = %q, want %q",
			got,
			secondSessionID,
		)
	}

	if got := manager.ActiveGeneration(); got != 2 {
		t.Fatalf(
			"second generation = %d, want 2",
			got,
		)
	}

	if status := manager.Status(); status.State != commands.OwnershipNotReady {
		t.Fatalf(
			"new connection must start NOT_READY: %+v",
			status,
		)
	}

	// A superseded connection must reject late snapshots
	// even before its WebSocket has been closed.
	lateSnapshotJSON, err := json.Marshal(
		ownershipIntegrationSnapshot(firstSessionID, 1),
	)
	if err != nil {
		t.Fatalf("marshal late ownership snapshot: %v", err)
	}

	err = first.ownership.applySnapshot(
		ctx,
		json.RawMessage(lateSnapshotJSON),
	)

	if !errors.Is(err, commands.ErrOwnershipConnectionSuperseded) &&
		!errors.Is(err, commands.ErrOwnershipConnectionInactive) {
		t.Fatalf(
			"expected superseded or inactive connection, got %v",
			err,
		)
	}

	if got := manager.ActiveGeneration(); got != 2 {
		t.Fatalf(
			"late snapshot changed active generation: %d",
			got,
		)
	}

	if status := manager.Status(); status.State != commands.OwnershipNotReady {
		t.Fatalf(
			"late snapshot changed new session ownership: %+v",
			status,
		)
	}

	// Closing the old WebSocket must not revoke the
	// newer connection's ownership generation.
	first.Close()

	if got := manager.ActiveGeneration(); got != 2 {
		t.Fatalf(
			"old connection revoked new generation: got %d, want 2",
			got,
		)
	}

	// The old connection must also be unable to apply
	// ownership snapshots after being superseded.
	snapshotJSON, err := json.Marshal(
		ownershipIntegrationSnapshot(firstSessionID, 1),
	)
	if err != nil {
		t.Fatalf("marshal old ownership snapshot: %v", err)
	}

	err = first.ownership.applySnapshot(
		ctx,
		json.RawMessage(snapshotJSON),
	)

	if err == nil {
		t.Fatal(
			"superseded ownership connection accepted snapshot",
		)
	}

	if !errors.Is(err, commands.ErrOwnershipConnectionClosed) &&
		!errors.Is(err, commands.ErrOwnershipConnectionSuperseded) &&
		!errors.Is(err, commands.ErrOwnershipConnectionInactive) {
		t.Fatalf(
			"unexpected superseded connection error: %v",
			err,
		)
	}

	if got := manager.ActiveGeneration(); got != 2 {
		t.Fatalf(
			"old snapshot changed active generation: %d",
			got,
		)
	}

	// Closing the current connection must revoke its
	// ownership authorization.
	second.Close()

	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf(
			"active generation after current close = %d, want 0",
			got,
		)
	}

	if status := manager.Status(); status.State != commands.OwnershipNotReady {
		t.Fatalf(
			"ownership must be NOT_READY after disconnect: %+v",
			status,
		)
	}
}
