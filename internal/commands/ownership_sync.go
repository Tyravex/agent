package commands

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrOwnershipNotReady = errors.New(
		"ownership synchronization is not ready",
	)
	ErrOwnershipSessionMismatch = errors.New(
		"ownership synchronization session mismatch",
	)
	ErrOwnershipSnapshotExpired = errors.New(
		"ownership synchronization snapshot expired",
	)
	ErrInvalidOwnershipSession = errors.New(
		"invalid ownership synchronization session",
	)
	ErrStaleOwnershipGeneration = errors.New(
		"stale ownership connection generation",
	)
	ErrOwnershipGenerationMismatch = errors.New(
		"ownership connection generation mismatch",
	)
)

const (
	maxOwnershipSessionIDLength = 128

	// Maximum validity period for a single ownership snapshot.
	// The Panel must periodically provide a newer snapshot
	// to maintain authorization.
	maxOwnershipSnapshotTTL = 5 * time.Minute
)

type OwnershipSyncState string

const (
	OwnershipNotReady OwnershipSyncState = "NOT_READY"
	OwnershipReady    OwnershipSyncState = "READY"
)

type OwnershipSyncStatus struct {
	State OwnershipSyncState

	SessionID string

	// Active connection generation. Zero means no active
	// generation-protected session.
	Generation uint64

	// Revision of the active snapshot.
	// Zero means no snapshot is currently active.
	Revision uint64

	// Highest accepted snapshot revision during this process.
	HighestRevision uint64

	// Snapshot expiration time.
	ExpiresAt time.Time
}

type OwnershipSyncManager struct {
	mu sync.RWMutex

	nodeID string

	sessionID string

	generation        uint64
	highestGeneration uint64

	revision        uint64
	highestRevision uint64

	expiresAt time.Time

	assignments map[string]ServiceOwnership

	now func() time.Time
}

var (
	_ ServiceOwnershipStore = (*OwnershipSyncManager)(nil)
	_ ServiceLeaseStore     = (*OwnershipSyncManager)(nil)
)

func NewOwnershipSyncManager(
	nodeID string,
) *OwnershipSyncManager {
	return &OwnershipSyncManager{
		nodeID:      nodeID,
		assignments: make(map[string]ServiceOwnership),
		now:         time.Now,
	}
}

// BeginSession starts a legacy session without a connection
// generation.
//
// Legacy sessions are supported for compatibility with
// existing tests and internal callers.
//
// Once generation-protected sessions have been used,
// this method is permanently disabled for this manager.
//
// The caller must authenticate the Panel connection before
// invoking this method. A session ID is not a credential.
func (m *OwnershipSyncManager) BeginSession(
	ctx context.Context,
	sessionID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if m == nil {
		return ErrAuthorizationUnavailable
	}

	if !validOwnershipSessionID(sessionID) {
		return ErrInvalidOwnershipSession
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if m.highestGeneration != 0 {
		return ErrStaleOwnershipGeneration
	}

	m.resetAuthorizationLocked()

	m.sessionID = sessionID

	return nil
}

// BeginSessionWithGeneration activates a session associated
// with an authenticated transport connection.
//
// Generations must be assigned by trusted transport code.
// Remote Panel messages must never choose their own
// generation.
//
// A newer generation invalidates previous authorization
// while preserving the highest accepted revision.
func (m *OwnershipSyncManager) BeginSessionWithGeneration(
	ctx context.Context,
	sessionID string,
	generation uint64,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if m == nil {
		return ErrAuthorizationUnavailable
	}

	if !validOwnershipSessionID(sessionID) {
		return ErrInvalidOwnershipSession
	}

	if generation == 0 {
		return ErrStaleOwnershipGeneration
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if generation <= m.highestGeneration {
		return ErrStaleOwnershipGeneration
	}

	m.resetAuthorizationLocked()

	m.highestGeneration = generation
	m.generation = generation
	m.sessionID = sessionID

	return nil
}

// ApplySnapshot replaces the active authorization snapshot
// for a legacy session.
//
// This method cannot modify a generation-protected session.
func (m *OwnershipSyncManager) ApplySnapshot(
	ctx context.Context,
	sessionID string,
	snapshot OwnershipSnapshot,
	ttl time.Duration,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if m == nil {
		return ErrAuthorizationUnavailable
	}

	if err := validateOwnershipTTL(ttl); err != nil {
		return err
	}

	replacement, err := validateOwnershipSnapshot(
		snapshot,
		m.nodeID,
	)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if m.highestGeneration != 0 {
		return ErrOwnershipGenerationMismatch
	}

	if m.sessionID == "" || sessionID != m.sessionID {
		return ErrOwnershipSessionMismatch
	}

	return m.applyValidatedSnapshotLocked(
		snapshot.Revision,
		replacement,
		ttl,
	)
}

// ApplySnapshotWithGeneration replaces the active
// authorization snapshot only for the matching
// generation-protected session.
//
// Authentication and transport-to-generation binding
// must be verified by the caller.
func (m *OwnershipSyncManager) ApplySnapshotWithGeneration(
	ctx context.Context,
	sessionID string,
	generation uint64,
	snapshot OwnershipSnapshot,
	ttl time.Duration,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if m == nil {
		return ErrAuthorizationUnavailable
	}

	if err := validateOwnershipTTL(ttl); err != nil {
		return err
	}

	replacement, err := validateOwnershipSnapshot(
		snapshot,
		m.nodeID,
	)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if generation == 0 ||
		m.generation == 0 ||
		generation != m.generation {
		return ErrOwnershipGenerationMismatch
	}

	if m.sessionID == "" || sessionID != m.sessionID {
		return ErrOwnershipSessionMismatch
	}

	// Revisions are monotonic within the authenticated connection generation.
	// The generation and session were verified above.
	if snapshot.Revision <= m.revision {
		return ErrStaleOwnershipRevision
	}

	now := m.now()

	m.assignments = replacement
	m.revision = snapshot.Revision
	m.expiresAt = now.Add(ttl)

	if snapshot.Revision > m.highestRevision {
		m.highestRevision = snapshot.Revision
	}

	return nil
}

// EndSession revokes authorization for a matching
// legacy session.
//
// It cannot revoke a generation-protected session.
func (m *OwnershipSyncManager) EndSession(
	sessionID string,
) {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.highestGeneration != 0 {
		return
	}

	if m.sessionID == "" || sessionID != m.sessionID {
		return
	}

	m.resetAuthorizationLocked()
}

// EndSessionWithGeneration revokes authorization only
// when both the session ID and generation match.
//
// Delayed disconnect events from older connections
// cannot revoke a newer session.
func (m *OwnershipSyncManager) EndSessionWithGeneration(
	sessionID string,
	generation uint64,
) {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if generation == 0 ||
		m.generation == 0 ||
		generation != m.generation ||
		m.sessionID == "" ||
		sessionID != m.sessionID {
		return
	}

	m.resetAuthorizationLocked()
}

// Status returns a consistent snapshot of the current
// authorization state.
func (m *OwnershipSyncManager) Status() OwnershipSyncStatus {
	if m == nil {
		return OwnershipSyncStatus{
			State: OwnershipNotReady,
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	status := OwnershipSyncStatus{
		State:           OwnershipNotReady,
		SessionID:       m.sessionID,
		Generation:      m.generation,
		Revision:        m.revision,
		HighestRevision: m.highestRevision,
		ExpiresAt:       m.expiresAt,
	}

	if m.readyLocked() {
		status.State = OwnershipReady
	}

	return status
}

// Lookup returns the ownership assignment for a service
// only when synchronization is READY.
//
// The returned assignment is a point-in-time result.
// Lifecycle execution should use WithAuthorizedService.
func (m *OwnershipSyncManager) Lookup(
	ctx context.Context,
	serviceID string,
) (ServiceOwnership, error) {
	if err := ctx.Err(); err != nil {
		return ServiceOwnership{}, err
	}

	if m == nil {
		return ServiceOwnership{}, ErrAuthorizationUnavailable
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return ServiceOwnership{}, err
	}

	if !m.readyLocked() {
		return ServiceOwnership{}, ErrOwnershipNotReady
	}

	if !authorizedServiceIDPattern.MatchString(serviceID) {
		return ServiceOwnership{}, fmt.Errorf(
			"%w: invalid service ID",
			ErrInvalidOwnership,
		)
	}

	ownership, ok := m.assignments[serviceID]
	if !ok {
		return ServiceOwnership{}, ErrOwnershipNotFound
	}

	return ownership, nil
}

// WithAuthorizedService authorizes a lifecycle operation
// at its start.
//
// Authorization is checked while holding the read lock.
// The lock is released before executing the callback.
//
// An operation already authorized may finish even if
// ownership is revoked or the session expires.
//
// The callback must execute only the single operation
// that was authorized.
func (m *OwnershipSyncManager) WithAuthorizedService(
	ctx context.Context,
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

	if nodeID != m.nodeID {
		m.mu.RUnlock()
		return ErrServiceNotAuthorized
	}

	if !m.readyLocked() {
		m.mu.RUnlock()
		return ErrOwnershipNotReady
	}

	if !authorizedServiceIDPattern.MatchString(serviceID) {
		m.mu.RUnlock()
		return ErrServiceNotAuthorized
	}

	ownership, exists := m.assignments[serviceID]

	if !exists ||
		!ownership.Enabled ||
		ownership.ServiceID != serviceID ||
		ownership.NodeID != nodeID {
		m.mu.RUnlock()
		return ErrServiceNotAuthorized
	}

	m.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	return run()
}

// readyLocked reports whether a current, unexpired
// authorization snapshot exists.
//
// The caller must hold m.mu.
func (m *OwnershipSyncManager) readyLocked() bool {
	return m.sessionID != "" &&
		m.revision > 0 &&
		m.now().Before(m.expiresAt)
}

// resetAuthorizationLocked removes active authorization
// without resetting the highest revision or generation.
//
// The caller must hold the write lock.
func (m *OwnershipSyncManager) resetAuthorizationLocked() {
	m.sessionID = ""
	m.generation = 0
	m.revision = 0
	m.expiresAt = time.Time{}
	m.assignments = make(map[string]ServiceOwnership)
}

// applyValidatedSnapshotLocked atomically publishes a
// validated snapshot.
//
// The caller must hold the write lock and verify the
// session and generation before invoking this method.
func (m *OwnershipSyncManager) applyValidatedSnapshotLocked(
	revision uint64,
	assignments map[string]ServiceOwnership,
	ttl time.Duration,
) error {
	if revision <= m.highestRevision {
		return ErrStaleOwnershipRevision
	}

	now := m.now()

	m.assignments = assignments
	m.revision = revision
	m.highestRevision = revision
	m.expiresAt = now.Add(ttl)

	return nil
}

func validOwnershipSessionID(sessionID string) bool {
	return sessionID != "" &&
		len(sessionID) <= maxOwnershipSessionIDLength
}

func validateOwnershipTTL(ttl time.Duration) error {
	if ttl <= 0 || ttl > maxOwnershipSnapshotTTL {
		return ErrOwnershipSnapshotExpired
	}

	return nil
}
