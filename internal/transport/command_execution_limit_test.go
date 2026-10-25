package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Tyravex/agent/internal/commands"
)

type sequentialExecutor struct {
	mu      sync.Mutex
	calls   []string
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newSequentialExecutor() *sequentialExecutor {
	return &sequentialExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (e *sequentialExecutor) Execute(
	ctx context.Context,
	command commands.Command,
) error {
	e.mu.Lock()
	e.calls = append(e.calls, command.ID)
	e.mu.Unlock()

	if command.ID == "first" {
		e.once.Do(func() {
			close(e.started)
		})

		select {
		case <-e.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (e *sequentialExecutor) callIDs() []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]string(nil), e.calls...)
}

func TestSessionLimitsConcurrentLifecycleCommands(t *testing.T) {
	executor := newSequentialExecutor()
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
				lifecycleCommandPayload("first"),
			); err != nil {
				serverDone <- err
				return
			}

			firstAccepted, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}
			if firstAccepted.Type != "command.accepted" {
				serverDone <- errors.New(
					"first command was not accepted",
				)
				return
			}

			select {
			case <-executor.started:
			case <-ctx.Done():
				serverDone <- errors.New(
					"first executor did not start",
				)
				return
			}

			if err := writeEnvelope(
				ctx,
				conn,
				"command.execute",
				lifecycleCommandPayload("second"),
			); err != nil {
				serverDone <- err
				return
			}

			secondAccepted, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}
			if secondAccepted.Type != "command.accepted" {
				serverDone <- errors.New(
					"second command was not acknowledged",
				)
				return
			}

			secondResult, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}
			if secondResult.Type != "command.result" {
				serverDone <- errors.New(
					"expected busy command.result",
				)
				return
			}

			var busy CommandResult
			if err := json.Unmarshal(
				secondResult.Payload,
				&busy,
			); err != nil {
				serverDone <- err
				return
			}

			if busy.CommandID != "second" ||
				busy.Success ||
				busy.ErrorCode != "COMMAND_BUSY" {
				serverDone <- errors.New(
					"unexpected busy response",
				)
				return
			}

			if got := executor.callIDs(); len(got) != 1 ||
				got[0] != "first" {
				serverDone <- errors.New(
					"second command was executed",
				)
				return
			}

			close(executor.release)

			firstResult, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}
			if firstResult.Type != "command.result" {
				serverDone <- errors.New(
					"expected first command result",
				)
				return
			}

			var completed CommandResult
			if err := json.Unmarshal(
				firstResult.Payload,
				&completed,
			); err != nil {
				serverDone <- err
				return
			}

			if !completed.Success ||
				completed.CommandID != "first" {
				serverDone <- errors.New(
					"first command did not complete",
				)
				return
			}

			// Wait until the execution slot has been released.
			// The result is sent before finishCommand runs,
			// so reading the result alone is not sufficient.
			serverDone <- nil

			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	session, err := sessionTestClient(server).
		ConnectWithExecutor(ctx, executor)
	if err != nil {
		t.Fatalf("ConnectWithExecutor: %v", err)
	}
	defer session.Close()

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("command limit test timed out")
	}

	if err := session.WaitCommands(ctx); err != nil {
		t.Fatalf("WaitCommands: %v", err)
	}

	if got := executor.callIDs(); len(got) != 1 ||
		got[0] != "first" {
		t.Fatalf("unexpected executed commands: %v", got)
	}
}

func TestWaitCommandsRespectsDeadline(t *testing.T) {
	executor := newBlockingCommandExecutor()

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

			sessionTestWelcome(t, ctx, conn)

			if err := writeEnvelope(
				ctx,
				conn,
				"command.execute",
				lifecycleCommandPayload("wait-command"),
			); err != nil {
				return
			}

			_, _, _ = conn.Read(ctx)
			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	session, err := sessionTestClient(server).
		ConnectWithExecutor(ctx, executor)
	if err != nil {
		t.Fatalf("ConnectWithExecutor: %v", err)
	}
	defer session.Close()

	select {
	case <-executor.started:
	case <-ctx.Done():
		t.Fatal("executor did not start")
	}

	waitCtx, waitCancel := context.WithTimeout(
		ctx,
		50*time.Millisecond,
	)
	defer waitCancel()

	err = session.WaitCommands(waitCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"expected deadline exceeded, got %v",
			err,
		)
	}

	close(executor.release)

	if err := session.WaitCommands(ctx); err != nil {
		t.Fatalf("command did not finish: %v", err)
	}
}

func TestSessionAcceptsCommandAfterPreviousCompletes(t *testing.T) {
	executor := newSequentialExecutor()
	serverDone := make(chan error, 1)
	allowNext := make(chan struct{})

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
				lifecycleCommandPayload("first"),
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
					"first command not accepted",
				)
				return
			}

			select {
			case <-executor.started:
			case <-ctx.Done():
				serverDone <- ctx.Err()
				return
			}

			close(executor.release)

			result, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}
			if result.Type != "command.result" {
				serverDone <- errors.New(
					"first command result missing",
				)
				return
			}

			// The test waits for WaitCommands before sending
			// the next command, avoiding a timing race.
			select {
			case <-allowNext:
			case <-ctx.Done():
				serverDone <- ctx.Err()
				return
			}

			if err := writeEnvelope(
				ctx,
				conn,
				"command.execute",
				lifecycleCommandPayload("third"),
			); err != nil {
				serverDone <- err
				return
			}

			nextAccepted, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}
			if nextAccepted.Type != "command.accepted" {
				serverDone <- errors.New(
					"next command not accepted",
				)
				return
			}

			nextResult, err := readEnvelope(ctx, conn)
			if err != nil {
				serverDone <- err
				return
			}
			if nextResult.Type != "command.result" {
				serverDone <- errors.New(
					"next command result missing",
				)
				return
			}

			var payload CommandResult
			if err := json.Unmarshal(
				nextResult.Payload,
				&payload,
			); err != nil {
				serverDone <- err
				return
			}

			if !payload.Success ||
				payload.CommandID != "third" {
				serverDone <- errors.New(
					"next command did not succeed",
				)
				return
			}

			serverDone <- nil

			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

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

	if err := session.WaitCommands(ctx); err != nil {
		t.Fatalf("WaitCommands: %v", err)
	}

	close(allowNext)

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("next command timed out")
	}

	if got := executor.callIDs(); len(got) != 2 ||
		got[0] != "first" ||
		got[1] != "third" {
		t.Fatalf(
			"unexpected execution order: %v",
			got,
		)
	}
}
