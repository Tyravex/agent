package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestConnectWithExecutorHandlesImmediateCommand(t *testing.T) {
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

			// Send welcome and immediately send a command.
			// The client has no opportunity to configure
			// its session after Connect returns.
			sessionTestWelcome(t, ctx, conn)

			err = writeEnvelope(
				ctx,
				conn,
				"command.execute",
				map[string]any{
					"commandId":  "immediate-command",
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

			var acceptedPayload CommandAccepted
			if err := json.Unmarshal(
				accepted.Payload,
				&acceptedPayload,
			); err != nil {
				serverDone <- err
				return
			}

			if acceptedPayload.CommandID != "immediate-command" {
				serverDone <- errors.New(
					"unexpected accepted command ID",
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

			var resultPayload CommandResult
			if err := json.Unmarshal(
				result.Payload,
				&resultPayload,
			); err != nil {
				serverDone <- err
				return
			}

			if !resultPayload.Success ||
				resultPayload.CommandID != "immediate-command" {
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
		t.Fatal("immediate command timed out")
	}

	if got := executor.callCount(); got != 1 {
		t.Fatalf(
			"expected one execution, got %d",
			got,
		)
	}
}

func TestConnectWithNilExecutorRejectsImmediateCommand(
	t *testing.T,
) {
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

			_ = writeEnvelope(
				ctx,
				conn,
				"command.execute",
				map[string]any{
					"commandId":  "unavailable-command",
					"operation":  "service.stop",
					"serviceId":  "service-1",
					"parameters": map[string]any{},
				},
			)

			// Wait for the Agent to close the connection.
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
		ConnectWithExecutor(ctx, nil)
	if err != nil {
		t.Fatalf("ConnectWithExecutor: %v", err)
	}
	defer session.Close()

	select {
	case <-session.Done():
	case <-ctx.Done():
		t.Fatal("session did not reject command")
	}

	if !errors.Is(
		session.Err(),
		ErrCommandExecutorUnavailable,
	) {
		t.Fatalf(
			"expected executor unavailable, got %v",
			session.Err(),
		)
	}
}
