package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tyravex/agent/internal/commands"
	"github.com/coder/websocket"
)

type blockingLifecycleExecutor struct {
	started chan struct{}
	release chan struct{}
	done    chan struct{}
}

func (e *blockingLifecycleExecutor) Execute(
	ctx context.Context,
	command commands.Command,
) error {
	close(e.started)

	// Intentionally ignore ctx cancellation.
	// Simulate an external operation that keeps running
	// after the WebSocket connection is closed.
	<-e.release

	close(e.done)
	return nil
}

func TestPersistentRegistryWaitsForExecutorAfterDisconnect(
	t *testing.T,
) {
	const commandID = "executor-after-disconnect-1"

	executor := &blockingLifecycleExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}

	// Ensure that a failed assertion cannot leave
	// the executor blocked forever.
	defer func() {
		select {
		case <-executor.release:
		default:
			close(executor.release)
		}
	}()

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
				lifecycleCommandPayload(commandID),
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
					"command was not accepted",
				)
				return
			}

			serverDone <- nil

			// Keep the server connection open.
			// The test will close the client session.
			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	cfg := sessionTestClient(server).config

	client, err := NewClientWithPersistentRegistry(
		cfg,
		path,
	)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.ClosePersistentRegistry()

	session, err := client.ConnectWithExecutor(
		ctx,
		executor,
	)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("command was not accepted")
	}

	select {
	case <-executor.started:
	case <-ctx.Done():
		t.Fatal("executor did not start")
	}

	// Close the WebSocket while the executor is blocked.
	session.Close()

	select {
	case <-session.Done():
	default:
		t.Fatal("session read loop did not stop")
	}

	select {
	case <-executor.done:
		t.Fatal("executor finished unexpectedly")
	default:
	}

	// The journal must remain locked even though
	// the WebSocket read loop has already stopped.
	if err := client.ClosePersistentRegistry(); !errors.Is(
		err,
		ErrClientBusy,
	) {
		t.Fatalf(
			"expected ErrClientBusy during execution, got %v",
			err,
		)
	}

	// Another client must not acquire the journal lock.
	other, err := NewClientWithPersistentRegistry(
		cfg,
		path,
	)
	if err == nil {
		_ = other.ClosePersistentRegistry()
		t.Fatal("second client acquired journal too early")
	}
	if !errors.Is(err, ErrCommandRegistryLocked) {
		t.Fatalf(
			"expected ErrCommandRegistryLocked, got %v",
			err,
		)
	}

	// Allow the external operation to complete.
	close(executor.release)

	select {
	case <-executor.done:
	case <-ctx.Done():
		t.Fatal("executor did not finish")
	}

	// WaitCommands also confirms that the command's
	// internal completion bookkeeping has finished.
	if err := session.WaitCommands(ctx); err != nil {
		t.Fatalf("wait for executor: %v", err)
	}

	if err := client.ClosePersistentRegistry(); err != nil {
		t.Fatalf(
			"close registry after executor: %v",
			err,
		)
	}

	// The journal should now be available to a new owner.
	reopened, err := NewClientWithPersistentRegistry(
		cfg,
		path,
	)
	if err != nil {
		t.Fatalf("reopen journal: %v", err)
	}
	defer reopened.ClosePersistentRegistry()

	// The ID was reserved before execution, so it
	// must remain unavailable after reopening.
	registry, ok := reopened.commandRegistry.(*persistentCommandRegistry)
	if !ok {
		t.Fatal("expected persistent registry")
	}

	if err := registry.reserve(commandID); !errors.Is(
		err,
		ErrDuplicateCommandID,
	) {
		t.Fatalf(
			"expected persisted duplicate, got %v",
			err,
		)
	}
}
