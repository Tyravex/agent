package panelws

import (
	"context"

	commands "github.com/Tyravex/agent/internal/command"
)

// generationLeaseStore binds authorization to the local
// ownership generation assigned to a WebSocket connection.
type generationLeaseStore struct {
	syncManager *commands.OwnershipSyncManager
	generation  uint64
}

func (s generationLeaseStore) WithAuthorizedService(
	ctx context.Context,
	nodeID string,
	serviceID string,
	run func() error,
) error {
	return s.syncManager.WithAuthorizedServiceForGeneration(
		ctx,
		s.generation,
		nodeID,
		serviceID,
		run,
	)
}

// ownershipExecutorBinder is installed only through the
// ownership-enabled connection entry point.
//
// The transport supplies the locally assigned generation,
// never a generation received from the Panel.
type ownershipExecutorBinder struct {
	nodeID      string
	syncManager *commands.OwnershipSyncManager
	runtime     commands.DockerServiceRuntime
}

func (b *ownershipExecutorBinder) BindOwnershipGeneration(
	generation uint64,
) CommandExecutor {
	return commands.NewLeasedDispatcher(
		b.nodeID,
		generationLeaseStore{
			syncManager: b.syncManager,
			generation:  generation,
		},
		commands.NewDockerExecutor(b.runtime),
	)
}

// Execute fails closed if a binder is used before it is bound
// to a locally assigned ownership generation.
func (b *ownershipExecutorBinder) Execute(
	ctx context.Context,
	command commands.Command,
) error {
	return commands.ErrOwnershipGenerationMismatch
}
