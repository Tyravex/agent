package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tyravex/agent/internal/runtime/docker"
	transport "github.com/Tyravex/agent/internal/transport/panelws"
	"github.com/coder/websocket"
)

func TestRunnerLocalOwnershipConnectWSS(t *testing.T) {
	const sessionID = "session-0000000000000001"

	server := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/agent/v1/ws" {
				http.NotFound(w, r)
				return
			}

			if r.Header.Get("X-Node-Id") != "node-1" ||
				r.Header.Get("Authorization") != "Bearer test-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

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

			_, hello, err := conn.Read(ctx)
			if err != nil {
				return
			}

			var envelope struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(hello, &envelope); err != nil {
				return
			}
			if envelope.Type != "node.hello" {
				return
			}

			welcome := map[string]any{
				"protocol":  1,
				"id":        "test-welcome-1",
				"type":      "node.welcome",
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				"payload": map[string]any{
					"nodeId":                   "node-1",
					"heartbeatIntervalSeconds": 30,
					"sessionId":                sessionID,
				},
			}

			data, err := json.Marshal(welcome)
			if err != nil {
				return
			}

			if err := conn.Write(
				ctx,
				websocket.MessageText,
				data,
			); err != nil {
				return
			}

			// Keep the test session alive until the client closes it.
			_, _, _ = conn.Read(ctx)
		},
	))
	defer server.Close()

	cfg := activationTestConfig()
	cfg.PanelURL = server.URL

	runner := NewRunner(cfg)

	if err := runner.configureLocalOwnershipExecution(); err != nil {
		if errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
			if _, hostErr := docker.LocalEngineHost(""); errors.Is(
				hostErr,
				docker.ErrLocalDockerEngineUnavailable,
			) {
				t.Skip("unsupported operating system")
			}
		}
		t.Fatal(err)
	}

	// Trust the test server certificate without disabling TLS checks.
	runner.transport = transport.NewClientWithHTTPClient(
		cfg,
		server.Client(),
	)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	session, err := runner.connectWithLocalOwnership(ctx)
	if err != nil {
		t.Fatalf("ownership WSS connection failed: %v", err)
	}
	defer session.Close()

	welcome := session.Welcome()

	if welcome.NodeID != "node-1" {
		t.Fatalf("unexpected node ID: %q", welcome.NodeID)
	}
	if welcome.SessionID != sessionID {
		t.Fatalf("unexpected session ID: %q", welcome.SessionID)
	}

	select {
	case <-session.Done():
		t.Fatalf("session closed prematurely: %v", session.Err())
	default:
	}

	session.Close()

	select {
	case <-session.Done():
	default:
		t.Fatal("session remained active after Close")
	}
}

func TestRunnerLocalOwnershipRejectsUntrustedTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("untrusted TLS connection reached HTTP handler")
			http.Error(w, "unexpected request", http.StatusForbidden)
		},
	))
	defer server.Close()

	cfg := activationTestConfig()
	cfg.PanelURL = server.URL

	runner := NewRunner(cfg)

	if err := runner.configureLocalOwnershipExecution(); err != nil {
		if errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
			if _, hostErr := docker.LocalEngineHost(""); errors.Is(
				hostErr,
				docker.ErrLocalDockerEngineUnavailable,
			) {
				t.Skip("unsupported operating system")
			}
		}
		t.Fatal(err)
	}

	// Deliberately retain the default HTTP client.
	// The test server's self-signed certificate is not trusted.
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	session, err := runner.connectWithLocalOwnership(ctx)
	if err == nil {
		if session != nil {
			session.Close()
		}
		t.Fatal("untrusted TLS certificate was accepted")
	}
	if session != nil {
		session.Close()
		t.Fatal("TLS failure must not return a session")
	}
}

func TestRunnerLocalOwnershipRejectsMutationBeforeWSS(t *testing.T) {
	requestReceived := make(chan struct{}, 1)

	server := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			select {
			case requestReceived <- struct{}{}:
			default:
			}
			http.Error(w, "unexpected request", http.StatusForbidden)
		},
	))
	defer server.Close()

	cfg := activationTestConfig()
	cfg.PanelURL = server.URL

	runner := NewRunner(cfg)

	if err := runner.configureLocalOwnershipExecution(); err != nil {
		if errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
			if _, hostErr := docker.LocalEngineHost(""); errors.Is(
				hostErr,
				docker.ErrLocalDockerEngineUnavailable,
			) {
				t.Skip("unsupported operating system")
			}
		}
		t.Fatal(err)
	}

	runner.transport = transport.NewClientWithHTTPClient(
		cfg,
		server.Client(),
	)

	// Changing the token after local initialization must
	// invalidate the ownership identity.
	runner.config.NodeToken = "mutated-token"

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	session, err := runner.connectWithLocalOwnership(ctx)

	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		if session != nil {
			session.Close()
		}
		t.Fatalf("expected ownership rejection, got %v", err)
	}
	if session != nil {
		session.Close()
		t.Fatal("mutated configuration returned a session")
	}

	select {
	case <-requestReceived:
		t.Fatal("mutated configuration reached the Panel")
	default:
	}
}
