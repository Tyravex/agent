package agent

import (
	"github.com/Tyravex/agent/internal/docker"
)

// configureLocalOwnershipExecution prepares a Docker runtime
// bound to the platform's supported local Docker Engine.
//
// It does not connect to Docker or the Panel.
// It must be called before Runner.Run(), not concurrently with it.
func (r *Runner) configureLocalOwnershipExecution() error {
	if r == nil || r.ownership != nil {
		return ErrRunnerOwnershipExecutionUnavailable
	}

	// Fail closed before creating the runtime.
	// The existing ownership factory performs the remaining
	// authorization and Panel TLS checks.
	if !r.config.DockerEnabled || r.config.DockerHost != "" {
		return ErrRunnerOwnershipExecutionUnavailable
	}

	host, err := docker.LocalEngineHost(r.config.DockerHost)
	if err != nil {
		return ErrRunnerOwnershipExecutionUnavailable
	}

	runtime, err := docker.NewRuntimeWithHost(host)
	if err != nil {
		return ErrRunnerOwnershipExecutionUnavailable
	}

	return r.configureOwnershipExecution(runtime)
}
