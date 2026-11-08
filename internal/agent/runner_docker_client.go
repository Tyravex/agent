package agent

import (
	"github.com/Tyravex/agent/internal/runtime/docker"
)

// newDockerStatusClient selects the SDK client used by heartbeats.
//
// Local ownership execution uses the same validated endpoint
// as its Docker CLI runtime. Legacy execution retains its
// existing SDK configuration behavior.
func (r *Runner) newDockerStatusClient() (*docker.Client, error) {
	if r.localDockerHost != "" {
		return docker.NewLocalClient(r.localDockerHost)
	}

	return docker.NewClientWithHost(r.config.DockerHost)
}
