package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/config"
	transport "github.com/Tyravex/agent/internal/transport/panelws"
)

func TestRunnerOwnershipConnectRejectsUnconfigured(t *testing.T) {
	runner := NewRunner(activationTestConfig())

	session, err := runner.connectWithLocalOwnership(context.Background())

	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if session != nil {
		t.Fatal("unconfigured Runner must not establish a session")
	}
}

func TestRunnerOwnershipConnectRejectsManualRuntime(t *testing.T) {
	runner := NewRunner(activationTestConfig())

	if err := runner.configureOwnershipExecution(
		&ownershipRuntimeStub{},
	); err != nil {
		t.Fatal(err)
	}

	session, err := runner.connectWithLocalOwnership(context.Background())

	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if session != nil {
		t.Fatal("manual runtime must not establish a session")
	}
}

func TestRunnerOwnershipConnectRejectsConfigurationMutation(t *testing.T) {
	runner := newIntegrityTestRunner(t)
	runner.config.NodeID = "other-node"

	session, err := runner.connectWithLocalOwnership(context.Background())

	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if session != nil {
		t.Fatal("mutated configuration must not establish a session")
	}
}

func TestRunnerOwnershipConnectRejectsForgedEndpoint(t *testing.T) {
	runner := newIntegrityTestRunner(t)
	runner.localDockerHost = "tcp://127.0.0.1:2375"

	session, err := runner.connectWithLocalOwnership(context.Background())

	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if session != nil {
		t.Fatal("forged endpoint must not establish a session")
	}
}

func TestNilRunnerOwnershipConnect(t *testing.T) {
	var runner *Runner

	session, err := runner.connectWithLocalOwnership(context.Background())

	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if session != nil {
		t.Fatal("nil Runner must not establish a session")
	}
}

func TestRunnerOwnershipConnectRejectsTransportMismatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{
			name: "different panel",
			mutate: func(cfg *config.Config) {
				cfg.PanelURL = "https://other-panel.example"
			},
		},
		{
			name: "different node",
			mutate: func(cfg *config.Config) {
				cfg.NodeID = "other-node"
			},
		},
		{
			name: "different token",
			mutate: func(cfg *config.Config) {
				cfg.NodeToken = "other-token"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newIntegrityTestRunner(t)

			transportConfig := runner.config
			tt.mutate(&transportConfig)

			runner.transport = transport.NewClient(transportConfig)

			// A canceled context prevents any network activity.
			// The guard must reject the mismatch before dialing.
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			session, err := runner.connectWithLocalOwnership(ctx)

			if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
				t.Fatalf("expected ownership rejection, got %v", err)
			}
			if session != nil {
				session.Close()
				t.Fatal("mismatched transport returned a session")
			}
		})
	}
}

func TestRunnerOwnershipConnectRejectsNilTransport(t *testing.T) {
	runner := newIntegrityTestRunner(t)
	runner.transport = nil

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	session, err := runner.connectWithLocalOwnership(ctx)

	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected ownership rejection, got %v", err)
	}
	if session != nil {
		session.Close()
		t.Fatal("nil transport returned a session")
	}
}
