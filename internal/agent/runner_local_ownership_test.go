package agent

import (
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/config"
)

func TestRunnerLocalOwnershipInitialization(t *testing.T) {
	runner := NewRunner(config.Config{
		PanelURL:      "https://panel.example",
		NodeID:        "node-1",
		NodeToken:     "test-token",
		DockerEnabled: true,
	})

	if runner.ownership != nil {
		t.Fatal("ownership must be disabled by default")
	}

	if err := runner.configureLocalOwnershipExecution(); err != nil {
		t.Fatal(err)
	}

	if runner.ownership == nil {
		t.Fatal("expected ownership execution")
	}

	original := runner.ownership

	err := runner.configureLocalOwnershipExecution()
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected duplicate initialization rejection, got %v", err)
	}

	if runner.ownership != original {
		t.Fatal("ownership execution was replaced")
	}
}

func TestRunnerLocalOwnershipRejectsInvalidConfiguration(t *testing.T) {
	valid := config.Config{
		PanelURL:      "https://panel.example",
		NodeID:        "node-1",
		NodeToken:     "test-token",
		DockerEnabled: true,
	}

	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{
			name: "docker disabled",
			mutate: func(c *config.Config) {
				c.DockerEnabled = false
			},
		},
		{
			name: "insecure panel",
			mutate: func(c *config.Config) {
				c.PanelURL = "http://localhost:8080"
			},
		},
		{
			name: "missing node ID",
			mutate: func(c *config.Config) {
				c.NodeID = ""
			},
		},
		{
			name: "missing token",
			mutate: func(c *config.Config) {
				c.NodeToken = ""
			},
		},
		{
			name: "remote Docker host",
			mutate: func(c *config.Config) {
				c.DockerHost = "tcp://127.0.0.1:2375"
			},
		},
		{
			name: "explicit local Docker host",
			mutate: func(c *config.Config) {
				c.DockerHost = "npipe:////./pipe/docker_engine"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.mutate(&cfg)

			runner := NewRunner(cfg)

			err := runner.configureLocalOwnershipExecution()
			if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
				t.Fatalf("expected unavailable error, got %v", err)
			}

			if runner.ownership != nil {
				t.Fatal("ownership must remain disabled")
			}
		})
	}
}

func TestNilRunnerLocalOwnershipInitialization(t *testing.T) {
	var runner *Runner

	err := runner.configureLocalOwnershipExecution()
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}
