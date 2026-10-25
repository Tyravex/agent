package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Tyravex/agent/internal/commands"
)

type cancellationExecutor struct {
	started  chan struct{}
	canceled chan struct{}
	once     sync.Once
}

func newCancellationExecutor() *cancellationExecutor {
	return &cancellationExecutor{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
}

func (e *cancellationExecutor) Execute(
	ctx context.Context,
	command commands.Command,
) error {
	e.once.Do(func() {
		close(e.started)
	})

	<-ctx.Done()
	close(e.canceled)

	return ctx.Err()
}

func TestCommandCanceledWhenPanelDisconnects(t *testing.T) {
	executor := newCancellationExecutor()
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
				lifecycleCommandPayload("cancel-command-1"),
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

			select {
			case <-executor.started:
			case <-ctx.Done():
				serverDone <- errors.New(
					"executor did not start",
				)
				return
			}

			// Simulate Panel disconnecting while the
			// command is still executing.
			conn.CloseNow()
			serverDone <- nil
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
		t.Fatal("server did not disconnect")
	}

	select {
	case <-session.Done():
	case <-ctx.Done():
		t.Fatal("Agent did not detect disconnection")
	}

	select {
	case <-executor.canceled:
	case <-ctx.Done():
		t.Fatal("executor did not receive cancellation")
	}
}

func TestCommandCanceledWhenSessionCloses(t *testing.T) {
	executor := newCancellationExecutor()
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
				lifecycleCommandPayload("cancel-command-2"),
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

			<-ctx.Done()
			serverDone <- nil
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

	select {
	case <-executor.started:
	case <-ctx.Done():
		session.Close()
		t.Fatal("executor did not start")
	}

	session.Close()

	select {
	case <-executor.canceled:
	case <-ctx.Done():
		t.Fatal("executor did not stop after session.Close")
	}
}
