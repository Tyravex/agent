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

type recordingExecutor struct {
	mu    sync.Mutex
	calls []commands.Command
	err   error
}

func (e *recordingExecutor) Execute(
	ctx context.Context,
	command commands.Command,
) error {
	e.mu.Lock()
	e.calls = append(e.calls, command)
	e.mu.Unlock()
	return e.err
}

func (e *recordingExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.calls)
}

func TestSessionExecutesLifecycleCommand(t *testing.T) {
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

			err = writeEnvelope(
				ctx,
				conn,
				"command.execute",
				map[string]any{
					"commandId":  "command-1",
					"operation":  "service.start",
					"serviceId":  "service-1",
					"parameters": map[string]any{},
				},
			)
			if err != nil {
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

			result, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}

			if result.Type != "command.result" {
				serverDone <- errors.New(
					"expected command.result",
				)
				return
			}

			var payload CommandResult
			if err := json.Unmarshal(
				result.Payload,
				&payload,
			); err != nil {
				serverDone <- err
				return
			}

			if !payload.Success ||
				payload.CommandID != "command-1" {
				serverDone <- errors.New(
					"unexpected command result",
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

	executor := &recordingExecutor{}
	session, err := sessionTestClient(server).Connect(ctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	if err := session.SetCommandExecutor(executor); err != nil {
		t.Fatalf("SetCommandExecutor: %v", err)
	}

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("command execution timed out")
	}

	if got := executor.callCount(); got != 1 {
		t.Fatalf("expected one execution, got %d", got)
	}
}

func TestSessionRejectsDuplicateCommandID(t *testing.T) {
	executor := &recordingExecutor{}

	state := commandExecutionState{
		executor: executor,
		seen: map[string]struct{}{
			"command-1": {},
		},
	}

	if len(state.seen) != 1 {
		t.Fatal("expected remembered command")
	}

	if _, exists := state.seen["command-1"]; !exists {
		t.Fatal("duplicate command ID was not remembered")
	}
}

func TestCommandFailureMessageIsSanitized(t *testing.T) {
	result, err := NewFailedCommandResult(
		"command-1",
		"EXECUTION_FAILED",
		"Service operation failed",
	)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result.Message, "docker") {
		t.Fatal("internal Docker details should not be exposed")
	}
}
