package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tyravex/agent/internal/config"
	"github.com/coder/websocket"
)

type integrationEnvelope struct {
	Protocol  int             `json:"protocol"`
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type integrationHeartbeat struct {
	UptimeSeconds int64 `json:"uptimeSeconds"`
	Docker        struct {
		Available bool    `json:"available"`
		Version   *string `json:"version"`
	} `json:"docker"`
}

func TestRunnerWebSocketReconnect(t *testing.T) {
	const (
		nodeID    = "integration-node"
		nodeToken = "integration-node-token"
	)

	var connections atomic.Int32

	heartbeats := make(chan int32, 4)
	serverErrors := make(chan error, 8)

	reportError := func(err error) {
		select {
		case serverErrors <- err:
		default:
		}
	}

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/agent/v1/ws" {
				reportError(fmt.Errorf(
					"unexpected WebSocket path: %s",
					r.URL.Path,
				))
				http.Error(w, "invalid path", http.StatusNotFound)
				return
			}

			if r.Header.Get("X-Node-Id") != nodeID {
				reportError(fmt.Errorf("invalid X-Node-Id header"))
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			if r.Header.Get("Authorization") != "Bearer "+nodeToken {
				reportError(fmt.Errorf("invalid Authorization header"))
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				reportError(fmt.Errorf("accept WebSocket: %w", err))
				return
			}
			defer conn.CloseNow()

			connectionNumber := connections.Add(1)

			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			defer cancel()

			_, helloData, err := conn.Read(ctx)
			if err != nil {
				reportError(fmt.Errorf("read hello: %w", err))
				return
			}

			var hello integrationEnvelope
			if err := json.Unmarshal(helloData, &hello); err != nil {
				reportError(fmt.Errorf("decode hello: %w", err))
				return
			}

			if hello.Protocol != 1 || hello.Type != "node.hello" {
				reportError(fmt.Errorf(
					"unexpected hello: protocol=%d type=%s",
					hello.Protocol,
					hello.Type,
				))
				return
			}

			var helloPayload struct {
				AgentVersion string   `json:"agentVersion"`
				Capabilities []string `json:"capabilities"`
			}

			if err := json.Unmarshal(
				hello.Payload,
				&helloPayload,
			); err != nil {
				reportError(fmt.Errorf("decode hello payload: %w", err))
				return
			}

			if helloPayload.AgentVersion == "" ||
				helloPayload.Capabilities == nil {
				reportError(fmt.Errorf("invalid hello payload"))
				return
			}

			welcome := map[string]any{
				"protocol":  1,
				"id":        "welcome-1",
				"type":      "node.welcome",
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				"payload": map[string]any{
					"nodeId":                   nodeID,
					"heartbeatIntervalSeconds": 5,
				},
			}

			welcomeData, err := json.Marshal(welcome)
			if err != nil {
				reportError(err)
				return
			}

			if err := conn.Write(
				ctx,
				websocket.MessageText,
				welcomeData,
			); err != nil {
				reportError(fmt.Errorf("write welcome: %w", err))
				return
			}

			_, heartbeatData, err := conn.Read(ctx)
			if err != nil {
				reportError(fmt.Errorf("read heartbeat: %w", err))
				return
			}

			var heartbeatEnvelope integrationEnvelope
			if err := json.Unmarshal(
				heartbeatData,
				&heartbeatEnvelope,
			); err != nil {
				reportError(fmt.Errorf("decode heartbeat: %w", err))
				return
			}

			if heartbeatEnvelope.Protocol != 1 ||
				heartbeatEnvelope.Type != "node.heartbeat" {
				reportError(fmt.Errorf(
					"unexpected heartbeat: %s",
					heartbeatEnvelope.Type,
				))
				return
			}

			var heartbeat integrationHeartbeat
			if err := json.Unmarshal(
				heartbeatEnvelope.Payload,
				&heartbeat,
			); err != nil {
				reportError(fmt.Errorf("decode heartbeat payload: %w", err))
				return
			}

			if heartbeat.UptimeSeconds < 0 {
				reportError(fmt.Errorf("negative uptime"))
				return
			}

			if heartbeat.Docker.Available ||
				heartbeat.Docker.Version != nil {
				reportError(fmt.Errorf(
					"unexpected Docker status when disabled",
				))
				return
			}

			ack := map[string]any{
				"protocol":  1,
				"id":        "ack-1",
				"type":      "node.heartbeat.ack",
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				"payload":   map[string]any{},
			}

			ackData, err := json.Marshal(ack)
			if err != nil {
				reportError(err)
				return
			}

			if err := conn.Write(
				ctx,
				websocket.MessageText,
				ackData,
			); err != nil {
				reportError(fmt.Errorf("write ACK: %w", err))
				return
			}

			select {
			case heartbeats <- connectionNumber:
			case <-ctx.Done():
				return
			}

			if connectionNumber == 1 {
				// Force a disconnect to verify automatic reconnection.
				return
			}

			// Keep the second session alive until Runner stops.
			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	cfg, err := config.FromValues(
		server.URL,
		nodeID,
		nodeToken,
		"30s",
	)
	if err != nil {
		t.Fatalf("create config: %v", err)
	}

	cfg.DockerEnabled = false

	ctx, cancel := context.WithCancel(context.Background())
	runnerDone := make(chan struct{})

	go func() {
		defer close(runnerDone)
		NewRunner(cfg).Run(ctx)
	}()

	defer func() {
		cancel()
		select {
		case <-runnerDone:
		case <-time.After(3 * time.Second):
			t.Error("Runner did not stop during cleanup")
		}
	}()

	timeout := time.NewTimer(8 * time.Second)
	defer timeout.Stop()

	for expected := int32(1); expected <= 2; expected++ {
		select {
		case err := <-serverErrors:
			t.Fatalf("mock Panel error: %v", err)

		case number := <-heartbeats:
			if number != expected {
				t.Fatalf(
					"expected heartbeat from connection %d, got %d",
					expected,
					number,
				)
			}

		case <-timeout.C:
			t.Fatalf(
				"timeout waiting for heartbeat from connection %d",
				expected,
			)
		}
	}

	if got := connections.Load(); got != 2 {
		t.Fatalf("expected 2 connections, got %d", got)
	}

	cancel()

	select {
	case <-runnerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Runner did not stop after cancellation")
	}

	select {
	case err := <-serverErrors:
		t.Fatalf("mock Panel error: %v", err)
	default:
	}

	// A stopped Runner must not initiate another connection.
	time.Sleep(150 * time.Millisecond)

	if got := connections.Load(); got != 2 {
		t.Fatalf(
			"expected no connections after shutdown, got %d",
			got,
		)
	}
}
