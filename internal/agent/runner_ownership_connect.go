package agent

import (
	"context"

	"github.com/Tyravex/agent/internal/transport"
)

// connectWithLocalOwnership validates local ownership authorization
// before attempting an ownership-enabled WebSocket connection.
//
// It must not be called concurrently with Runner configuration.
// Production Runner.Run does not call this method.
func (r *Runner) connectWithLocalOwnership(
	ctx context.Context,
) (*transport.Session, error) {
	execution, err := r.localOwnershipExecution()
	if err != nil {
		return nil, err
	}

	return r.transport.ConnectWithOwnershipExecution(ctx, execution)
}
