package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestPersistentCommandIDRejectedAfterClientRestart(t *testing.T) {
	const commandID = "persistent-restart-1"

	executor := &recordingExecutor{}
	var connections atomic.Int32

	serverDone := make(chan error, 2)

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			number := connections.Add(1)

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
				lifecycleCommandPayload(commandID),
			); err != nil {
				serverDone <- err
				return
			}

			if number == 1 {
				accepted, err := readEnvelope(ctx, conn)
				if err != nil {
					serverDone <- err
					return
				}

				if accepted.Type != "command.accepted" {
					serverDone <- fmt.Errorf(
						"expected command.accepted, got %s",
						accepted.Type,
					)
					return
				}

				result, err := readEnvelope(ctx, conn)
				if err != nil {
					serverDone <- err
					return
				}

				if result.Type != "command.result" {
					serverDone <- fmt.Errorf(
						"expected command.result, got %s",
						result.Type,
					)
					return
				}

				var completed CommandResult
				if err := decodeStrict(
					result.Payload,
					&completed,
				); err != nil {
					serverDone <- err
					return
				}

				if !completed.Success ||
					completed.CommandID != commandID {
					serverDone <- errors.New(
						"first command did not succeed",
					)
					return
				}

				serverDone <- nil

				// Keep the connection open until the
				// first client closes its session.
				_, _, _ = conn.Read(ctx)
				return
			}

			// A duplicate must close the session without
			// an accepted or result message.
			_, err = readEnvelope(ctx, conn)
			if err == nil {
				serverDone <- errors.New(
					"replayed command received a response",
				)
				return
			}

			serverDone <- nil
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

	firstClient, err := NewClientWithPersistentRegistry(
		cfg,
		path,
	)
	if err != nil {
		t.Fatalf("create first client: %v", err)
	}
	defer firstClient.ClosePersistentRegistry()

	firstSession, err := firstClient.ConnectWithExecutor(
		ctx,
		executor,
	)
	if err != nil {
		t.Fatalf("first connection: %v", err)
	}
	defer firstSession.Close()

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("first command timed out")
	}

	if err := firstSession.WaitCommands(ctx); err != nil {
		t.Fatalf("wait for first command: %v", err)
	}

	firstSession.Close()

	if got := executor.callCount(); got != 1 {
		t.Fatalf(
			"expected one execution, got %d",
			got,
		)
	}

	if err := firstClient.ClosePersistentRegistry(); err != nil {
		t.Fatalf("close first registry: %v", err)
	}

	// Simulate a process restart by constructing a new
	// Client and reopening the existing journal.
	secondClient, err := NewClientWithPersistentRegistry(
		cfg,
		path,
	)
	if err != nil {
		t.Fatalf("create second client: %v", err)
	}
	defer secondClient.ClosePersistentRegistry()

	secondSession, err := secondClient.ConnectWithExecutor(
		ctx,
		executor,
	)
	if err != nil {
		t.Fatalf("second connection: %v", err)
	}
	defer secondSession.Close()

	select {
	case <-secondSession.Done():
	case <-ctx.Done():
		t.Fatal("duplicate command was not rejected")
	}

	if err := secondSession.Err(); err == nil ||
		!strings.Contains(
			err.Error(),
			"duplicate command ID",
		) {
		t.Fatalf(
			"expected duplicate command ID error, got %v",
			err,
		)
	}

	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("second connection did not close")
	}

	if got := executor.callCount(); got != 1 {
		t.Fatalf(
			"replayed command was executed: %d calls",
			got,
		)
	}

	if got := connections.Load(); got != 2 {
		t.Fatalf(
			"expected two WebSocket connections, got %d",
			got,
		)
	}
}
