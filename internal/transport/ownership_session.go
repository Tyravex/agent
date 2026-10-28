package transport

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Tyravex/agent/internal/commands"
)

var (
	ErrOwnershipTransportDisabled = errors.New(
		"ownership transport is not enabled",
	)

	ErrOwnershipTransportNotActivated = errors.New(
		"ownership transport session is not activated",
	)
)

// ownershipSession binds a WebSocket session to one
// locally generated ownership connection.
//
// Creating this object does not establish trust.
// Activation must happen only after the transport has
// independently verified the Panel's identity.
type ownershipSession struct {
	connection *commands.OwnershipConnection
}

// newOwnershipSession allocates a local ownership
// connection but does not activate it.
func newOwnershipSession(
	ctx context.Context,
	manager *commands.OwnershipConnectionManager,
) (*ownershipSession, error) {
	if manager == nil {
		return nil, ErrOwnershipTransportDisabled
	}

	connection, err := manager.Open(ctx)
	if err != nil {
		return nil, err
	}

	return &ownershipSession{
		connection: connection,
	}, nil
}

// activate must only be called by trusted transport
// lifecycle code, never directly in response to an
// untrusted WebSocket payload.
func (s *ownershipSession) activate(
	ctx context.Context,
	sessionID string,
) error {
	if s == nil || s.connection == nil {
		return ErrOwnershipTransportDisabled
	}

	return s.connection.Activate(ctx, sessionID)
}

// applySnapshot passes the received payload through
// the connection-bound ownership authorization layer.
//
// A snapshot cannot activate an inactive connection.
func (s *ownershipSession) applySnapshot(
	ctx context.Context,
	payload json.RawMessage,
) error {
	if s == nil || s.connection == nil {
		return ErrOwnershipTransportDisabled
	}

	return s.connection.ApplySnapshot(ctx, payload)
}

// close revokes the current ownership session.
//
// Closing a superseded connection does not revoke
// ownership belonging to a newer connection.
func (s *ownershipSession) close() {
	if s == nil || s.connection == nil {
		return
	}

	s.connection.Close()
}

func (s *ownershipSession) checkActive(
	ctx context.Context,
) error {
	if s == nil || s.connection == nil {
		return ErrOwnershipTransportDisabled
	}

	return s.connection.CheckActive(ctx)
}
