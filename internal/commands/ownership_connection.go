package commands

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrOwnershipConnectionUnavailable = errors.New(
		"ownership connection manager unavailable",
	)

	ErrOwnershipConnectionInactive = errors.New(
		"ownership connection is not active",
	)

	ErrOwnershipConnectionSuperseded = errors.New(
		"ownership connection has been superseded",
	)

	ErrOwnershipConnectionClosed = errors.New(
		"ownership connection is closed",
	)

	ErrOwnershipConnectionGenerationExhausted = errors.New(
		"ownership connection generation exhausted",
	)
)

// OwnershipConnectionManager binds ownership authorization
// to a locally created connection identity.
//
// The manager does not authenticate a remote Panel.
// Its Activate method must only be called after trusted
// transport code has established the Panel's identity.
//
// A manager owns its sync manager exclusively. Do not call
// BeginSession, ApplySnapshot or EndSession directly on the
// underlying sync manager while using this component.
type OwnershipConnectionManager struct {
	mu sync.Mutex

	syncManager *OwnershipSyncManager

	nextGeneration uint64
	active         *OwnershipConnection
}

// OwnershipConnection represents one local transport
// connection. Its generation is assigned by the Agent,
// never by the remote Panel.
type OwnershipConnection struct {
	manager *OwnershipConnectionManager

	generation uint64

	sessionID string
	activated bool
	closed    bool
}

// NewOwnershipConnectionManager creates a connection
// manager for an existing ownership synchronization manager.
//
// A nil sync manager is rejected when operations are called.
func NewOwnershipConnectionManager(
	syncManager *OwnershipSyncManager,
) *OwnershipConnectionManager {
	return &OwnershipConnectionManager{
		syncManager: syncManager,
	}
}

// Open creates a new local connection identity.
//
// Opening a newer connection does not immediately revoke
// the current one. Authorization is replaced atomically
// when the newer connection is activated.
//
// This permits a transport to establish and authenticate
// a replacement connection before switching ownership.
func (m *OwnershipConnectionManager) Open(
	ctx context.Context,
) (*OwnershipConnection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if m == nil || m.syncManager == nil {
		return nil, ErrOwnershipConnectionUnavailable
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if m.nextGeneration == ^uint64(0) {
		return nil, ErrOwnershipConnectionGenerationExhausted
	}

	m.nextGeneration++

	return &OwnershipConnection{
		manager:    m,
		generation: m.nextGeneration,
	}, nil
}

// Generation returns the locally assigned connection
// generation. It is informational and must not be used
// as proof of authentication.
func (c *OwnershipConnection) Generation() uint64 {
	if c == nil {
		return 0
	}

	return c.generation
}

// Activate binds a verified connection to an ownership
// session.
//
// SECURITY: Only trusted transport code may call Activate.
// A received node.welcome or untrusted JSON field must not
// independently trigger this method.
//
// An older connection cannot activate after a newer
// connection has already been activated.
//
// Activation always resets authorization to NOT_READY.
// A fresh snapshot is required before service operations.
func (c *OwnershipConnection) Activate(
	ctx context.Context,
	sessionID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if c == nil || c.manager == nil {
		return ErrOwnershipConnectionUnavailable
	}

	m := c.manager

	if m.syncManager == nil {
		return ErrOwnershipConnectionUnavailable
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if c.closed {
		return ErrOwnershipConnectionClosed
	}

	if c.activated {
		return ErrStaleOwnershipGeneration
	}

	if !validOwnershipSessionID(sessionID) {
		return ErrInvalidOwnershipSession
	}

	if m.active != nil &&
		c.generation <= m.active.generation {
		return ErrOwnershipConnectionSuperseded
	}

	// The sync manager performs the authoritative
	// generation high-watermark check and resets
	// authorization before a new snapshot arrives.
	if err := m.syncManager.BeginSessionWithGeneration(
		ctx,
		sessionID,
		c.generation,
	); err != nil {
		return err
	}

	if m.active != nil {
		m.active.activated = false
	}

	c.sessionID = sessionID
	c.activated = true
	m.active = c

	return nil
}

// CheckActive verifies that this exact connection
// still owns the active ownership generation.
//
// It does not activate ownership or extend a lease.
// A successful result does not mean that a valid
// ownership snapshot is READY.
func (c *OwnershipConnection) CheckActive(
	ctx context.Context,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if c == nil || c.manager == nil {
		return ErrOwnershipConnectionUnavailable
	}

	m := c.manager

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if m.syncManager == nil {
		return ErrOwnershipConnectionUnavailable
	}

	if c.closed {
		return ErrOwnershipConnectionClosed
	}

	if m.active != c {
		if m.active != nil &&
			m.active.generation > c.generation {
			return ErrOwnershipConnectionSuperseded
		}

		return ErrOwnershipConnectionInactive
	}

	if !c.activated {
		return ErrOwnershipConnectionInactive
	}

	return nil
}

// ApplySnapshot accepts a complete JSON ownership message
// only from the currently activated connection.
//
// The message's session_id must match the session that
// was bound to this connection during Activate.
//
// The manager mutex serializes activation, snapshot
// application and disconnect processing. Consequently,
// an old connection cannot apply a snapshot after a
// newer connection has taken over.
func (c *OwnershipConnection) ApplySnapshot(
	ctx context.Context,
	data []byte,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if c == nil || c.manager == nil {
		return ErrOwnershipConnectionUnavailable
	}

	m := c.manager

	if m.syncManager == nil {
		return ErrOwnershipConnectionUnavailable
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if c.closed {
		return ErrOwnershipConnectionClosed
	}

	if m.active != c {
		if c.activated {
			return ErrOwnershipConnectionSuperseded
		}

		return ErrOwnershipConnectionInactive
	}

	if !c.activated {
		return ErrOwnershipConnectionInactive
	}

	message, err := DecodeOwnershipSnapshotMessage(data)
	if err != nil {
		return err
	}

	if message.SessionID != c.sessionID {
		return ErrOwnershipSessionMismatch
	}

	// ApplyOwnershipSnapshotMessage validates the message
	// again and performs the authoritative snapshot
	// revision, generation, node and TTL checks.
	//
	// The second decode is intentionally conservative:
	// correctness is preferred over avoiding one bounded
	// JSON parse in this initial integration layer.
	return ApplyOwnershipSnapshotMessage(
		ctx,
		m.syncManager,
		c.generation,
		data,
	)
}

// Close invalidates authorization only if this connection
// is currently active.
//
// A delayed disconnect from an older connection must not
// revoke ownership belonging to a newer connection.
func (c *OwnershipConnection) Close() {
	if c == nil || c.manager == nil {
		return
	}

	m := c.manager

	m.mu.Lock()
	defer m.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true

	if m.active != c {
		c.activated = false
		return
	}

	if m.syncManager != nil {
		m.syncManager.EndSessionWithGeneration(
			c.sessionID,
			c.generation,
		)
	}

	c.activated = false
	m.active = nil
}

// ActiveGeneration reports the generation of the current
// activated connection.
//
// A zero value means there is no active connection.
func (m *OwnershipConnectionManager) ActiveGeneration() uint64 {
	if m == nil {
		return 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return 0
	}

	return m.active.generation
}

// Status exposes the underlying ownership sync status.
//
// It does not expose connection authentication state.
// READY only means the current sync manager holds a valid
// snapshot within its TTL.
func (m *OwnershipConnectionManager) Status() OwnershipSyncStatus {
	if m == nil || m.syncManager == nil {
		return OwnershipSyncStatus{
			State: OwnershipNotReady,
		}
	}

	return m.syncManager.Status()
}
