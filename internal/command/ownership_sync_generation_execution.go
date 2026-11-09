package commands

import (
	"context"

	"github.com/Tyravex/agent/internal/ownership"
)

// WithAuthorizedServiceForGeneration authorizes an operation
// only for the exact locally assigned connection generation.
//
// The generation and service assignment are checked under
// the same read lock. The callback runs after unlocking.
func (m *OwnershipSyncManager) WithAuthorizedServiceForGeneration(
	ctx context.Context,
	generation uint64,
	nodeID string,
	serviceID string,
	run func() error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if m == nil || run == nil {
		return ErrLeaseUnavailable
	}

	m.mu.RLock()

	if err := ctx.Err(); err != nil {
		m.mu.RUnlock()
		return err
	}

	if generation == 0 ||
		m.generation != generation {
		m.mu.RUnlock()
		return ErrOwnershipGenerationMismatch
	}

	if nodeID != m.nodeID {
		m.mu.RUnlock()
		return ErrServiceNotAuthorized
	}

	if !m.readyLocked() {
		m.mu.RUnlock()
		return ErrOwnershipNotReady
	}

	if !ownership.ValidServiceID(serviceID) {
		m.mu.RUnlock()
		return ErrServiceNotAuthorized
	}

	assignment, exists := m.assignments[serviceID]

	if !exists ||
		!assignment.Enabled ||
		assignment.ServiceID != serviceID ||
		assignment.NodeID != nodeID {
		m.mu.RUnlock()
		return ErrServiceNotAuthorized
	}

	m.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	return run()
}
