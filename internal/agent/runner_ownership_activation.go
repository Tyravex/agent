package agent

import (
	"github.com/Tyravex/agent/internal/docker"
	"github.com/Tyravex/agent/internal/transport"
)

// localOwnershipExecution returns an ownership executor only when
// the Runner was configured with a supported local Docker endpoint.
//
// It does not connect to the Panel or Docker Engine.
// It must not be called concurrently with Runner configuration.
func (r *Runner) localOwnershipExecution() (*transport.OwnershipExecution, error) {
	if r == nil ||
		r.ownership == nil ||
		r.localDockerHost == "" ||
		!r.config.DockerEnabled ||
		r.config.DockerHost != "" {
		return nil, ErrRunnerOwnershipExecutionUnavailable
	}

	expectedHost, err := docker.LocalEngineHost(r.config.DockerHost)
	if err != nil || r.localDockerHost != expectedHost {
		return nil, ErrRunnerOwnershipExecutionUnavailable
	}

	return r.ownership, nil
}
