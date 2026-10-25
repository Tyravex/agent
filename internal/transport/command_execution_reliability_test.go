package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Tyravex/agent/internal/commands"
)

type blockingCommandExecutor struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingCommandExecutor() *blockingCommandExecutor {
	return &blockingCommandExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (e *blockingCommandExecutor) Execute(
	ctx context.Context,
	command commands.Command,
) error {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()

	e.once.Do(func() {
		close(e.started)
	})

	select {
	case <-e.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *blockingCommandExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func lifecycleCommandPayload(commandID string) map[string]any {
	return map[string]any{
		"commandId":  commandID,
		"operation":  "service.start",
		"serviceId":  "service-1",
		"parameters": map[string]any{},
	}
}

func TestSessionRejectsDuplicateCommandOverWebSocket(t *testing.T) {
	serverDone := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				serverDone <- err
				return
			}
			defer conn.CloseNow()

			ctx, cancel := context.WithTimeout(
				r.Context(),
				5*time.Second,
			)
			defer cancel()

			sessionTestWelcome(t, ctx, conn)

			command := lifecycleCommandPayload("duplicate-1")

			if err := writeEnvelope(
				ctx,
				conn,
				"command.execute",
				command,
			); err != nil {
				serverDone <- err
				return
			}

			accepted, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}

			if accepted.Type != "command.accepted" {
				serverDone <- errors.New(
					"first command was not accepted",
				)
				return
			}

			// Send the same command ID again.
			if err := writeEnvelope(
				ctx,
				conn,
				"command.execute",
				command,
			); err != nil {
				serverDone <- err
				return
			}

			// The Agent should close the session rather than
			// acknowledge or execute a duplicate command.
			_, _, err = conn.Read(ctx)

			if err == nil {
				serverDone <- errors.New(
					"expected connection closure after duplicate",
				)
				return
			}

			serverDone <- nil
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	executor := newBlockingCommandExecutor()
	defer close(executor.release)

	session, err := sessionTestClient(server).
		ConnectWithExecutor(ctx, executor)
	if err != nil {
		t.Fatalf("ConnectWithExecutor: %v", err)
	}
	defer session.Close()

	select {
	case <-executor.started:
	case <-ctx.Done():
		t.Fatal("first command did not start")
	}

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("duplicate command test timed out")
	}

	if got := executor.callCount(); got != 1 {
		t.Fatalf(
			"duplicate command executed: %d calls",
			got,
		)
	}

	if err := session.Err(); err == nil ||
		!strings.Contains(err.Error(), "duplicate command ID") {
		t.Fatalf(
			"expected duplicate command rejection, got %v",
			err,
		)
	}
}

func TestHeartbeatDuringRunningCommand(t *testing.T) {
	serverDone := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				serverDone <- err
				return
			}
			defer conn.CloseNow()

			ctx, cancel := context.WithTimeout(
				r.Context(),
				5*time.Second,
			)
			defer cancel()

			sessionTestWelcome(t, ctx, conn)

			if err := writeEnvelope(
				ctx,
				conn,
				"command.execute",
				lifecycleCommandPayload("long-running-1"),
			); err != nil {
				serverDone <- err
				return
			}

			accepted, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}

			if accepted.Type != "command.accepted" {
				serverDone <- errors.New(
					"expected command.accepted",
				)
				return
			}

			heartbeat, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}

			if heartbeat.Type != "node.heartbeat" {
				serverDone <- errors.New(
					"expected heartbeat during command",
				)
				return
			}

			if err := writeEnvelope(
				ctx,
				conn,
				"node.heartbeat.ack",
				struct{}{},
			); err != nil {
				serverDone <- err
				return
			}

			// The command remains blocked at this point.
			serverDone <- nil

			// Wait until the Agent closes the connection.
			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	executor := newBlockingCommandExecutor()
	defer close(executor.release)

	session, err := sessionTestClient(server).
		ConnectWithExecutor(ctx, executor)
	if err != nil {
		t.Fatalf("ConnectWithExecutor: %v", err)
	}
	defer session.Close()

	select {
	case <-executor.started:
	case <-ctx.Done():
		t.Fatal("command did not start")
	}

	heartbeatCtx, heartbeatCancel := context.WithTimeout(
		ctx,
		2*time.Second,
	)
	defer heartbeatCancel()

	if err := session.SendHeartbeat(
		heartbeatCtx,
		Heartbeat{UptimeSeconds: 42},
	); err != nil {
		t.Fatalf(
			"heartbeat failed during command: %v",
			err,
		)
	}

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("heartbeat test timed out")
	}

	if got := executor.callCount(); got != 1 {
		t.Fatalf(
			"expected one running command, got %d",
			got,
		)
	}
}

func TestCommandResultDoesNotExposeExecutorError(t *testing.T) {
	secret := "secret-token-do-not-expose"

	result, err := NewFailedCommandResult(
		"command-1",
		"EXECUTION_FAILED",
		"Service operation failed",
	)
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(encoded), secret) {
		t.Fatal("command result contains private executor details")
	}
}
