package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestCommandIDRejectedAfterReconnect(t *testing.T) {
	executor := &recordingExecutor{}

	var connections atomic.Int32
	serverDone := make(chan error, 2)

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			connectionNumber := connections.Add(1)

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
				lifecycleCommandPayload("reconnect-1"),
			); err != nil {
				serverDone <- err
				return
			}

			if connectionNumber == 1 {
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

				var completed CommandResult
				if err := json.Unmarshal(
					result.Payload,
					&completed,
				); err != nil {
					serverDone <- err
					return
				}

				if !completed.Success ||
					completed.CommandID != "reconnect-1" {
					serverDone <- errors.New(
						"first command did not succeed",
					)
					return
				}

				serverDone <- nil

				// Keep the first connection open until
				// the client closes it.
				_, _, _ = conn.Read(ctx)
				return
			}

			// A replayed command must close the second session
			// without being acknowledged or executed.
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

	client := sessionTestClient(server)

	first, err := client.ConnectWithExecutor(ctx, executor)
	if err != nil {
		t.Fatalf("first ConnectWithExecutor: %v", err)
	}

	select {
	case err := <-serverDone:
		if err != nil {
			first.Close()
			t.Fatal(err)
		}

	case <-ctx.Done():
		first.Close()
		t.Fatal("first command timed out")
	}

	if err := first.WaitCommands(ctx); err != nil {
		first.Close()
		t.Fatalf("first WaitCommands: %v", err)
	}

	first.Close()

	if got := executor.callCount(); got != 1 {
		t.Fatalf(
			"expected one execution before reconnect, got %d",
			got,
		)
	}

	second, err := client.ConnectWithExecutor(ctx, executor)
	if err != nil {
		t.Fatalf("second ConnectWithExecutor: %v", err)
	}
	defer second.Close()

	select {
	case <-second.Done():
	case <-ctx.Done():
		t.Fatal("replayed command was not rejected")
	}

	if err := second.Err(); err == nil ||
		!strings.Contains(err.Error(), "duplicate command ID") {
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
}
