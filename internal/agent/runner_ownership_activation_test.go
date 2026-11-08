package agent

import (
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/runtime/docker"
)

func activationTestConfig() config.Config {
	return config.Config{
		PanelURL:      "https://panel.example",
		NodeID:        "node-1",
		NodeToken:     "test-token",
		DockerEnabled: true,
	}
}

func TestRunnerOwnershipActivationRejectsUnconfigured(t *testing.T) {
	runner := NewRunner(activationTestConfig())

	execution, err := runner.localOwnershipExecution()
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if execution != nil {
		t.Fatal("unconfigured Runner must not expose ownership")
	}
}

func TestRunnerOwnershipActivationRejectsManualRuntime(t *testing.T) {
	runner := NewRunner(activationTestConfig())

	if err := runner.configureOwnershipExecution(
		&ownershipRuntimeStub{},
	); err != nil {
		t.Fatal(err)
	}

	execution, err := runner.localOwnershipExecution()
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if execution != nil {
		t.Fatal("manual runtime must not pass local activation")
	}
}

func TestRunnerOwnershipActivationAcceptsLocalRuntime(t *testing.T) {
	runner := NewRunner(activationTestConfig())

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

	execution, err := runner.localOwnershipExecution()
	if err != nil {
		t.Fatal(err)
	}
	if execution == nil || execution != runner.ownership {
		t.Fatal("local activation must return the existing ownership")
	}
}

func TestRunnerOwnershipActivationRejectsEndpointMismatch(t *testing.T) {
	runner := NewRunner(activationTestConfig())

	if err := runner.configureLocalOwnershipExecution(); err != nil {
		if _, hostErr := docker.LocalEngineHost(""); errors.Is(
			hostErr,
			docker.ErrLocalDockerEngineUnavailable,
		) {
			t.Skip("unsupported operating system")
		}
		t.Fatal(err)
	}

	runner.localDockerHost = "tcp://127.0.0.1:2375"

	execution, err := runner.localOwnershipExecution()
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if execution != nil {
		t.Fatal("mismatched endpoint must not pass activation")
	}
}

func TestNilRunnerOwnershipActivation(t *testing.T) {
	var runner *Runner

	execution, err := runner.localOwnershipExecution()
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if execution != nil {
		t.Fatal("nil Runner must not expose ownership")
	}
}
