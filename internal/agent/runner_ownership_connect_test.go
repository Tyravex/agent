package agent

import (
	"context"
	"errors"
	"testing"
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
