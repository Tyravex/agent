package agent

import (
	"github.com/Tyravex/agent/internal/commands"
)

// configureOwnershipExecution prepares one ownership execution
// environment for the lifetime of this Runner.
//
// It does not establish a WebSocket connection or execute Docker commands.
func (r *Runner) configureOwnershipExecution(
	runtime commands.DockerServiceRuntime,
) error {
	if r == nil {
		return ErrRunnerOwnershipExecutionUnavailable
	}

	// Never replace an existing manager: its ownership generations
	// must survive WebSocket reconnects.
	if r.ownership != nil {
		return ErrRunnerOwnershipExecutionUnavailable
	}

	execution, err := prepareOwnershipExecution(r.config, runtime)
	if err != nil {
		return err
	}

	r.ownership = execution
	return nil
}
