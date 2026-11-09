package panelws

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	commands "github.com/Tyravex/agent/internal/command"
)

var ErrOwnershipExecutionUnavailable = errors.New(
	"ownership execution is unavailable",
)

// OwnershipExecution binds connection management and lifecycle
// execution to the same ownership synchronization manager.
//
// Keep this object alive across WebSocket reconnects.
type OwnershipExecution struct {
	nodeID      string
	connections *commands.OwnershipConnectionManager
	dispatcher  *commands.LeasedDispatcher
}

// NewOwnershipExecution creates a shared ownership execution
// environment. It does not connect to the Panel or execute Docker
// operations.
func NewOwnershipExecution(
	nodeID string,
	syncManager *commands.OwnershipSyncManager,
	runtime commands.DockerServiceRuntime,
) (*OwnershipExecution, error) {
	if nodeID == "" ||
		syncManager == nil ||
		syncManager.NodeID() != nodeID ||
		runtime == nil {
		return nil, ErrOwnershipExecutionUnavailable
	}

	return &OwnershipExecution{
		nodeID: nodeID,
		connections: commands.NewOwnershipConnectionManager(
			syncManager,
		),
		dispatcher: commands.NewLeasedDispatcher(
			nodeID,
			syncManager,
			commands.NewDockerExecutor(runtime),
		),
	}, nil
}

// ConnectWithOwnershipExecution connects using the same ownership
// state for synchronization and lifecycle authorization.
func (c *Client) ConnectWithOwnershipExecution(
	ctx context.Context,
	execution *OwnershipExecution,
) (*Session, error) {
	if execution == nil ||
		execution.connections == nil ||
		execution.dispatcher == nil ||
		execution.nodeID == "" {
		return nil, ErrOwnershipExecutionUnavailable
	}

	// Reuse the existing WSS requirement.
	// Do not permit ownership over an unverified transport.
	return c.connectWithOwnershipExecution(ctx, execution)
}

// connectWithOwnershipExecution validates the transport before
// installing an ownership-bound command executor.
func (c *Client) connectWithOwnershipExecution(
	ctx context.Context,
	execution *OwnershipExecution,
) (*Session, error) {
	if c == nil ||
		execution == nil ||
		execution.connections == nil ||
		execution.dispatcher == nil ||
		execution.nodeID != c.config.NodeID {
		return nil, ErrOwnershipExecutionUnavailable
	}

	parsed, err := url.Parse(c.config.PanelURL)
	if err != nil {
		return nil, fmt.Errorf("invalid panel URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return nil, ErrOwnershipRequiresTLS
	}

	return c.connect(
		ctx,
		execution.dispatcher,
		execution.connections,
	)
}
