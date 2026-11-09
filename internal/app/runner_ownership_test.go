package app

import (
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/config"
)

func TestRunnerOwnershipInitialization(t *testing.T) {
	runner := NewRunner(config.Config{
		PanelURL:      "https://panel.example",
		NodeID:        "node-1",
		NodeToken:     "test-token",
		DockerEnabled: true,
	})

	if runner.ownership != nil {
		t.Fatal("ownership must be disabled by default")
	}

	if err := runner.configureOwnershipExecution(
		&ownershipRuntimeStub{},
	); err != nil {
		t.Fatal(err)
	}

	if runner.ownership == nil {
		t.Fatal("expected ownership execution")
	}

	original := runner.ownership

	err := runner.configureOwnershipExecution(
		&ownershipRuntimeStub{},
	)
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected duplicate initialization rejection, got %v", err)
	}

	if runner.ownership != original {
		t.Fatal("ownership execution was replaced")
	}
}

func TestRunnerOwnershipDisabled(t *testing.T) {
	runner := NewRunner(config.Config{
		PanelURL:      "https://panel.example",
		NodeID:        "node-1",
		NodeToken:     "test-token",
		DockerEnabled: false,
	})

	err := runner.configureOwnershipExecution(
		&ownershipRuntimeStub{},
	)
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected disabled ownership rejection, got %v", err)
	}

	if runner.ownership != nil {
		t.Fatal("ownership must remain disabled")
	}
}

func TestNilRunnerOwnershipInitialization(t *testing.T) {
	var runner *Runner

	err := runner.configureOwnershipExecution(
		&ownershipRuntimeStub{},
	)
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}
