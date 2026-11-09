package panelws

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

	commands "github.com/Tyravex/agent/internal/command"
	"github.com/coder/websocket"
)

func TestOwnershipExecutionRejectsCommandFromSupersededWebSocket(t *testing.T) {
	const (
		oldSessionID = "session-0000000000000001"
		newSessionID = "session-0000000000000002"
	)

	var connectionNumber atomic.Uint64
	sendOldCommand := make(chan struct{})
	oldCommandOutcome := make(chan error, 1)

	server := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()

			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()

			hello, err := readEnvelope(ctx, conn)
			if err != nil || hello.Type != "node.hello" {
				return
			}

			number := connectionNumber.Add(1)
			sessionID := fmt.Sprintf("session-%016d", number)

			if err := writeEnvelope(ctx, conn, "node.welcome", Welcome{
				NodeID:                   "node-1",
				HeartbeatIntervalSeconds: 30,
				SessionID:                sessionID,
			}); err != nil {
				return
			}

			if number == 2 {
				if err := writeEnvelope(
					ctx,
					conn,
					"ownership.snapshot",
					ownershipIntegrationSnapshot(newSessionID, 1),
				); err != nil {
					return
				}

				// Keep the newer WebSocket connected.
				_, _, _ = conn.Read(ctx)
				return
			}

			if number != 1 {
				return
			}

			select {
			case <-sendOldCommand:
			case <-ctx.Done():
				oldCommandOutcome <- ctx.Err()
				return
			}

			payload := json.RawMessage(`{
"commandId":"command-from-superseded-ws",
"operation":"service.start",
"serviceId":"minecraft-1",
"parameters":{}
}`)

			if err := writeEnvelope(
				ctx,
				conn,
				"command.execute",
				payload,
			); err != nil {
				oldCommandOutcome <- err
				return
			}

			for {
				message, err := readEnvelope(ctx, conn)
				if err != nil {
					// Closing the superseded connection is acceptable.
					oldCommandOutcome <- nil
					return
				}

				if message.Type != "command.result" {
					continue
				}

				var result struct {
					Success bool `json:"success"`
				}
				if err := json.Unmarshal(message.Payload, &result); err != nil {
					oldCommandOutcome <- err
					return
				}

				if result.Success {
					oldCommandOutcome <- errors.New(
						"superseded WebSocket executed a command successfully",
					)
					return
				}

				oldCommandOutcome <- nil
				return
			}
		},
	))
	defer server.Close()

	runtime := &recordingOwnershipRuntime{}
	syncManager := commands.NewOwnershipSyncManager("node-1")

	execution, err := NewOwnershipExecution(
		"node-1",
		syncManager,
		runtime,
	)
	if err != nil {
		t.Fatal(err)
	}

	client := ownershipIntegrationClient(server)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	oldSession, err := client.ConnectWithOwnershipExecution(ctx, execution)
	if err != nil {
		t.Fatal(err)
	}
	defer oldSession.Close()

	if oldSession.Welcome().SessionID != oldSessionID {
		t.Fatalf("unexpected old session ID: %q", oldSession.Welcome().SessionID)
	}

	newSession, err := client.ConnectWithOwnershipExecution(ctx, execution)
	if err != nil {
		t.Fatal(err)
	}
	defer newSession.Close()

	if newSession.Welcome().SessionID != newSessionID {
		t.Fatalf("unexpected new session ID: %q", newSession.Welcome().SessionID)
	}

	waitOwnershipReady(t, ctx, newSession, execution.connections)

	if got := execution.connections.ActiveGeneration(); got != 2 {
		t.Fatalf("expected generation 2, got %d", got)
	}

	close(sendOldCommand)

	select {
	case err := <-oldCommandOutcome:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for superseded WebSocket outcome")
	}

	if got := runtime.callCount(); got != 0 {
		t.Fatalf(
			"superseded WebSocket executed %d runtime operations",
			got,
		)
	}
}
