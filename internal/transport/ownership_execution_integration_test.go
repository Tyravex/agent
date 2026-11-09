package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	commands "github.com/Tyravex/agent/internal/command"
	"github.com/coder/websocket"
)

type recordingOwnershipRuntime struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingOwnershipRuntime) Start(
	ctx context.Context,
	serviceID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, "start:"+serviceID)
	return nil
}

func (r *recordingOwnershipRuntime) Stop(
	ctx context.Context,
	serviceID string,
) error {
	return errors.New("unexpected stop")
}

func (r *recordingOwnershipRuntime) Restart(
	ctx context.Context,
	serviceID string,
) error {
	return errors.New("unexpected restart")
}

func (r *recordingOwnershipRuntime) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func TestOwnershipExecutionWebSocketDeniesBeforeReady(t *testing.T) {
	resultReceived := make(chan error, 1)

	server := ownershipIntegrationServer(
		t,
		func(ctx context.Context, conn *websocket.Conn) {
			payload := json.RawMessage(`{
                "commandId":"command-before-ready",
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
				resultReceived <- err
				return
			}

			accepted, err := readEnvelope(ctx, conn)
			if err != nil {
				resultReceived <- err
				return
			}
			if accepted.Type != "command.accepted" {
				resultReceived <- errors.New("expected command.accepted")
				return
			}

			result, err := readEnvelope(ctx, conn)
			if err != nil {
				resultReceived <- err
				return
			}
			if result.Type != "command.result" {
				resultReceived <- errors.New("expected command.result")
				return
			}

			var body struct {
				CommandID string `json:"commandId"`
				Success   bool   `json:"success"`
				ErrorCode string `json:"errorCode"`
			}

			if err := json.Unmarshal(result.Payload, &body); err != nil {
				resultReceived <- err
				return
			}

			if body.CommandID != "command-before-ready" {
				resultReceived <- errors.New("unexpected command ID")
				return
			}
			if body.Success {
				resultReceived <- errors.New("command succeeded before READY")
				return
			}
			if body.ErrorCode != "EXECUTION_FAILED" {
				resultReceived <- fmt.Errorf(
					"expected EXECUTION_FAILED, got payload: %s",
					result.Payload,
				)
				return
			}

			resultReceived <- nil
		},
	)
	defer server.Close()

	syncManager := commands.NewOwnershipSyncManager("node-1")
	runtime := &recordingOwnershipRuntime{}

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
		5*time.Second,
	)
	defer cancel()

	session, err := client.ConnectWithOwnershipExecution(
		ctx,
		execution,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	select {
	case err := <-resultReceived:
		if err != nil {
			t.Fatalf("command protocol: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for command.result")
	}

	if got := runtime.callCount(); got != 0 {
		t.Fatalf("command executed before READY: %d calls", got)
	}

	if got := execution.connections.Status().State; got != commands.OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}
}

func TestOwnershipExecutionWebSocketStartsOwnedServiceWhenReady(t *testing.T) {
	commandResult := make(chan error, 1)
	sendCommand := make(chan struct{})

	server := ownershipIntegrationServer(
		t,
		func(ctx context.Context, conn *websocket.Conn) {
			if err := writeEnvelope(
				ctx,
				conn,
				"ownership.snapshot",
				ownershipIntegrationSnapshot(ownershipIntegrationSessionID, 1),
			); err != nil {
				commandResult <- err
				return
			}

			select {
			case <-sendCommand:
			case <-ctx.Done():
				commandResult <- ctx.Err()
				return
			}

			if err := writeEnvelope(
				ctx,
				conn,
				"command.execute",
				json.RawMessage(`{
                    "commandId":"command-ready",
                    "operation":"service.start",
                    "serviceId":"minecraft-1",
                    "parameters":{}
                }`),
			); err != nil {
				commandResult <- err
				return
			}

			accepted, err := readEnvelope(ctx, conn)
			if err != nil {
				commandResult <- err
				return
			}
			if accepted.Type != "command.accepted" {
				commandResult <- errors.New("expected command.accepted")
				return
			}

			result, err := readEnvelope(ctx, conn)
			if err != nil {
				commandResult <- err
				return
			}
			if result.Type != "command.result" {
				commandResult <- errors.New("expected command.result")
				return
			}

			var body struct {
				CommandID string `json:"commandId"`
				Success   bool   `json:"success"`
				ErrorCode string `json:"errorCode"`
			}

			if err := json.Unmarshal(result.Payload, &body); err != nil {
				commandResult <- err
				return
			}
			if body.CommandID != "command-ready" {
				commandResult <- errors.New("unexpected command ID")
				return
			}
			if !body.Success || body.ErrorCode != "" {
				commandResult <- fmt.Errorf(
					"expected successful execution, got %s",
					result.Payload,
				)
				return
			}

			commandResult <- nil
		},
	)
	defer server.Close()

	syncManager := commands.NewOwnershipSyncManager("node-1")
	runtime := &recordingOwnershipRuntime{}

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
		5*time.Second,
	)
	defer cancel()

	session, err := client.ConnectWithOwnershipExecution(ctx, execution)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	waitOwnershipReady(t, ctx, session, execution.connections)
	close(sendCommand)

	select {
	case err := <-commandResult:
		if err != nil {
			t.Fatalf("command protocol: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for command.result")
	}

	if got := runtime.callCount(); got != 1 {
		t.Fatalf("expected exactly one runtime call, got %d", got)
	}
}

func TestOwnershipExecutionWebSocketRevokesOnDisconnect(t *testing.T) {
	server := ownershipIntegrationServer(
		t,
		func(ctx context.Context, conn *websocket.Conn) {
			if err := writeEnvelope(
				ctx,
				conn,
				"ownership.snapshot",
				ownershipIntegrationSnapshot(ownershipIntegrationSessionID, 1),
			); err != nil {
				return
			}

			// Keep the connection alive until the client closes it.
			_, _, _ = conn.Read(ctx)
		},
	)
	defer server.Close()

	syncManager := commands.NewOwnershipSyncManager("node-1")
	runtime := &recordingOwnershipRuntime{}

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
		5*time.Second,
	)
	defer cancel()

	session, err := client.ConnectWithOwnershipExecution(ctx, execution)
	if err != nil {
		t.Fatal(err)
	}

	waitOwnershipReady(t, ctx, session, execution.connections)

	session.Close()

	command := commands.Command{
		ID:        "command-after-disconnect",
		ServiceID: "minecraft-1",
		Operation: commands.ServiceStart,
	}

	if err := execution.dispatcher.Execute(ctx, command); err == nil {
		t.Fatal("command was authorized after disconnect")
	}

	if got := runtime.callCount(); got != 0 {
		t.Fatalf("runtime executed after disconnect: %d calls", got)
	}

	if got := execution.connections.Status().State; got != commands.OwnershipNotReady {
		t.Fatalf("expected NOT_READY after disconnect, got %s", got)
	}
}
