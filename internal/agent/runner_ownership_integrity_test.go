package agent

import (
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/docker"
)

func newIntegrityTestRunner(t *testing.T) *Runner {
	t.Helper()

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

	return runner
}

func requireActivationRejected(t *testing.T, runner *Runner) {
	t.Helper()

	execution, err := runner.localOwnershipExecution()
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if execution != nil {
		t.Fatal("activation must not expose ownership")
	}
}

func TestRunnerOwnershipActivationRejectsConfigurationMutation(t *testing.T) {
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
			name: "remote Docker host",
			mutate: func(c *config.Config) {
				c.DockerHost = "tcp://127.0.0.1:2375"
			},
		},
		{
			name: "insecure panel",
			mutate: func(c *config.Config) {
				c.PanelURL = "http://panel.example"
			},
		},
		{
			name: "changed panel",
			mutate: func(c *config.Config) {
				c.PanelURL = "https://other.example"
			},
		},
		{
			name: "changed node ID",
			mutate: func(c *config.Config) {
				c.NodeID = "other-node"
			},
		},
		{
			name: "missing node token",
			mutate: func(c *config.Config) {
				c.NodeToken = ""
			},
		},
		{
			name: "changed node token",
			mutate: func(c *config.Config) {
				c.NodeToken = "other-token"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := newIntegrityTestRunner(t)

			tc.mutate(&runner.config)

			requireActivationRejected(t, runner)
		})
	}
}

func TestRunnerOwnershipActivationRejectsMissingOwnership(t *testing.T) {
	runner := newIntegrityTestRunner(t)
	runner.ownership = nil

	requireActivationRejected(t, runner)
}

func TestRunnerOwnershipActivationRejectsMissingEndpoint(t *testing.T) {
	runner := newIntegrityTestRunner(t)
	runner.localDockerHost = ""

	requireActivationRejected(t, runner)
}

func TestRunnerOwnershipActivationRejectsManualRuntimeAfterLocalFailure(t *testing.T) {
	runner := NewRunner(activationTestConfig())

	runner.config.PanelURL = "http://panel.example"

	if err := runner.configureLocalOwnershipExecution(); !errors.Is(
		err,
		ErrRunnerOwnershipExecutionUnavailable,
	) {
		t.Fatalf("expected local initialization rejection, got %v", err)
	}

	runner.config.PanelURL = "https://panel.example"

	if err := runner.configureOwnershipExecution(
		&ownershipRuntimeStub{},
	); err != nil {
		t.Fatal(err)
	}

	requireActivationRejected(t, runner)
}

func TestRunnerOwnershipActivationPreservesOriginalOwnership(t *testing.T) {
	runner := newIntegrityTestRunner(t)

	original := runner.ownership
	originalHost := runner.localDockerHost

	err := runner.configureLocalOwnershipExecution()
	if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
		t.Fatalf("expected duplicate initialization rejection, got %v", err)
	}

	if runner.ownership != original {
		t.Fatal("duplicate initialization replaced ownership")
	}
	if runner.localDockerHost != originalHost {
		t.Fatal("duplicate initialization changed endpoint")
	}

	execution, err := runner.localOwnershipExecution()
	if err != nil {
		t.Fatal(err)
	}
	if execution != original {
		t.Fatal("activation returned a different ownership instance")
	}
}

func TestRunnerOwnershipActivationRejectsForgedLocalEndpoint(t *testing.T) {
	runner := NewRunner(activationTestConfig())

	if err := runner.configureOwnershipExecution(
		&ownershipRuntimeStub{},
	); err != nil {
		t.Fatal(err)
	}

	host, err := docker.LocalEngineHost("")
	if errors.Is(err, docker.ErrLocalDockerEngineUnavailable) {
		t.Skip("unsupported operating system")
	}
	if err != nil {
		t.Fatal(err)
	}

	// A valid-looking endpoint does not prove that the
	// ownership runtime was created by the local factory.
	runner.localDockerHost = host

	if runner.localOwnershipIdentity != nil {
		t.Fatal("manual runtime must not have local ownership identity")
	}

	requireActivationRejected(t, runner)
}
