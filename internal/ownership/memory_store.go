package ownership

import (
	"context"
	"fmt"
	"sync"
)

// MemoryOwnershipStore is an in-memory implementation
// of ServiceOwnershipStore.
//
// It is intended for authorization tests and as a
// foundation for a future trusted synchronization layer.
//
// Assignments are not persisted across Agent restarts.
type MemoryOwnershipStore struct {
	mu          sync.RWMutex
	assignments map[string]ServiceOwnership

	snapshotRevision uint64
	snapshotApplied  bool
}

var _ ServiceOwnershipStore = (*MemoryOwnershipStore)(nil)

// NewMemoryOwnershipStore creates an empty registry.
//
// An empty registry denies all service operations.
func NewMemoryOwnershipStore() *MemoryOwnershipStore {
	return &MemoryOwnershipStore{
		assignments: make(map[string]ServiceOwnership),
	}
}

// Lookup returns a copy of the current assignment.
//
// It never creates an assignment. A missing service
// is reported as ErrOwnershipNotFound.
func (s *MemoryOwnershipStore) Lookup(
	ctx context.Context,
	serviceID string,
) (ServiceOwnership, error) {
	if err := ctx.Err(); err != nil {
		return ServiceOwnership{}, err
	}

	if s == nil {
		return ServiceOwnership{}, ErrAuthorizationUnavailable
	}

	if !ValidServiceID(serviceID) {
		return ServiceOwnership{}, fmt.Errorf(
			"%w: invalid service ID",
			ErrInvalidOwnership,
		)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return ServiceOwnership{}, err
	}

	ownership, exists := s.assignments[serviceID]
	if !exists {
		return ServiceOwnership{}, ErrOwnershipNotFound
	}

	return ownership, nil
}

// Upsert replaces the assignment for a service.
//
// This method must only be called by trusted Agent-side
// code. Never expose it directly to command.execute.
func (s *MemoryOwnershipStore) Upsert(
	ctx context.Context,
	ownership ServiceOwnership,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if s == nil {
		return ErrAuthorizationUnavailable
	}

	if err := ValidateAssignment(ownership); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if s.assignments == nil {
		s.assignments = make(
			map[string]ServiceOwnership,
		)
	}

	s.assignments[ownership.ServiceID] = ownership
	return nil
}

// Revoke removes an assignment.
//
// Revoking a missing service is idempotent.
func (s *MemoryOwnershipStore) Revoke(
	ctx context.Context,
	serviceID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if s == nil {
		return ErrAuthorizationUnavailable
	}

	if !ValidServiceID(serviceID) {
		return fmt.Errorf(
			"%w: invalid service ID",
			ErrInvalidOwnership,
		)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	delete(s.assignments, serviceID)
	return nil
}

var _ ServiceLeaseStore = (*MemoryOwnershipStore)(nil)

// WithAuthorizedService holds a read lock from the
// ownership check until run returns.
//
// Upsert and Revoke require the write lock, so neither
// can change the assignment during an active callback.
//
// The callback must not call Upsert or Revoke on this
// store: doing so would deadlock.
func (s *MemoryOwnershipStore) WithAuthorizedService(
	ctx context.Context,
	nodeID string,
	serviceID string,
	run func() error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if s == nil {
		return ErrLeaseUnavailable
	}

	if nodeID == "" ||
		!ValidServiceID(serviceID) ||
		run == nil {
		return ErrServiceNotAuthorized
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	ownership, exists := s.assignments[serviceID]
	if !exists ||
		!ownership.Enabled ||
		ownership.ServiceID != serviceID ||
		ownership.NodeID != nodeID {
		return ErrServiceNotAuthorized
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	return run()
}

// ApplySnapshot atomically replaces all assignments.
//
// The caller must provide a complete snapshot from an
// authenticated, trusted synchronization source.
//
// A newer snapshot waits for active ownership leases.
// Once applied, services absent from the snapshot are
// no longer authorized.
//
// Revision is only remembered in memory. This method
// must not be exposed as a production synchronization
// endpoint until restart and replay protection exist.
func (s *MemoryOwnershipStore) ApplySnapshot(
	ctx context.Context,
	expectedNodeID string,
	snapshot OwnershipSnapshot,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if s == nil {
		return ErrAuthorizationUnavailable
	}

	// Validate and construct the replacement map before
	// acquiring the write lock. No partial update is possible.
	replacement, err := ValidateSnapshot(
		snapshot,
		expectedNodeID,
	)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if s.snapshotApplied &&
		snapshot.Revision <= s.snapshotRevision {
		return ErrStaleOwnershipRevision
	}

	s.assignments = replacement
	s.snapshotRevision = snapshot.Revision
	s.snapshotApplied = true

	return nil
}

// SnapshotRevision reports the latest accepted revision.
//
// The boolean is false until the first snapshot has been
// applied. The returned revision is not durable.
func (s *MemoryOwnershipStore) SnapshotRevision() (
	uint64,
	bool,
) {
	if s == nil {
		return 0, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshotRevision, s.snapshotApplied
}

// RestoreRevision restores a previously persisted revision
// without restoring any service assignments.
//
// The caller must use this method only during trusted
// store initialization, before the store is shared.
func (s *MemoryOwnershipStore) RestoreRevision(
	revision uint64,
	applied bool,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshotRevision = revision
	s.snapshotApplied = applied
}

// PublishPersistedSnapshot installs a snapshot that has
// already been validated and durably recorded.
//
// The caller must serialize this operation with all
// persistent-store operations and prevent new leases
// until publication completes.
//
// This method does not perform disk I/O.
func (s *MemoryOwnershipStore) PublishPersistedSnapshot(
	revision uint64,
	assignments map[string]ServiceOwnership,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.assignments = assignments
	s.snapshotRevision = revision
	s.snapshotApplied = true
}
