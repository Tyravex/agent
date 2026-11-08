package agent

import (
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/docker"
)

func TestRunnerLocalOwnershipPinsDockerEndpoint(t *testing.T) {
	cfg := config.Config{
		PanelURL:      "https://panel.example",
		NodeID:        "node-1",
		NodeToken:     "test-token",
		DockerEnabled: true,
	}

	expected, err := docker.LocalEngineHost("")
	if errors.Is(err, docker.ErrLocalDockerEngineUnavailable) {
		t.Skip("unsupported operating system")
	}
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")
	t.Setenv("DOCKER_CONTEXT", "remote")
	t.Setenv("DOCKER_TLS_VERIFY", "1")
	t.Setenv("DOCKER_CERT_PATH", "nonexistent")

	runner := NewRunner(cfg)

	if runner.localDockerHost != "" {
		t.Fatal("local Docker endpoint must not be configured by default")
	}

	if err := runner.configureLocalOwnershipExecution(); err != nil {
		t.Fatal(err)
	}

	if runner.localDockerHost != expected {
		t.Fatalf(
			"local endpoint = %q, want %q",
			runner.localDockerHost,
			expected,
		)
	}

	client, err := runner.newDockerStatusClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if got := client.DaemonHost(); got != expected {
		t.Fatalf("SDK endpoint = %q, want %q", got, expected)
	}
}

func TestRunnerManualOwnershipDoesNotPinDockerEndpoint(t *testing.T) {
	runner := NewRunner(config.Config{
		PanelURL:      "https://panel.example",
		NodeID:        "node-1",
		NodeToken:     "test-token",
		DockerEnabled: true,
	})

	if err := runner.configureOwnershipExecution(
		&ownershipRuntimeStub{},
	); err != nil {
		t.Fatal(err)
	}

	if runner.localDockerHost != "" {
		t.Fatal("manual ownership must not claim a local endpoint")
	}
}

func TestRunnerFailedLocalOwnershipDoesNotPinEndpoint(t *testing.T) {
	runner := NewRunner(config.Config{
		PanelURL:      "http://localhost:8080",
		NodeID:        "node-1",
		NodeToken:     "test-token",
		DockerEnabled: true,
	})

	err := runner.configureLocalOwnershipExecution()
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}

	if runner.localDockerHost != "" {
		t.Fatal("failed initialization must not pin an endpoint")
	}

	if runner.ownership != nil {
		t.Fatal("failed initialization must not create ownership")
	}
}
