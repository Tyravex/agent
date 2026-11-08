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
	if r == nil ||
		r.ownership != nil ||
		r.localDockerHost != "" ||
		r.localOwnershipIdentity != nil {
		return ErrRunnerOwnershipExecutionUnavailable
	}

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

	if err := r.configureOwnershipExecution(runtime); err != nil {
		return err
	}

	identity := ownershipIdentityFromConfig(r.config)

	// Publish local activation state only after successful initialization.
	r.localDockerHost = host
	r.localOwnershipIdentity = &identity

	return nil
}
