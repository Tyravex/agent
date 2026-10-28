package transport

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/Tyravex/agent/internal/commands"
)

var (
	ErrOwnershipRequiresTLS = errors.New(
		"ownership transport requires verified WSS",
	)

	ErrOwnershipSessionUnavailable = errors.New(
		"ownership is not configured for this websocket session",
	)

	ErrOwnershipSessionIDMismatch = errors.New(
		"ownership sessionId does not match node.welcome",
	)
)

// ConnectWithOwnership establishes a WebSocket session with
// ownership support, but leaves ownership NOT_READY.
//
// Unlike ConnectWithExecutor, this method never installs
// a command executor.
//
// A caller must explicitly activate ownership after
// establishing a trusted ownership session contract
// with the Panel.
func (c *Client) ConnectWithOwnership(
	ctx context.Context,
	manager *commands.OwnershipConnectionManager,
) (*Session, error) {
	if manager == nil {
		return nil, ErrOwnershipTransportDisabled
	}

	parsed, err := url.Parse(c.config.PanelURL)
	if err != nil {
		return nil, fmt.Errorf("invalid panel URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return nil, ErrOwnershipRequiresTLS
	}

	return c.connect(ctx, nil, manager)
}

// ActivateOwnership binds the ownership authorization
// state to this particular WebSocket connection.
//
// SECURITY: The caller must obtain sessionID through
// a trusted protocol exchange. Never activate ownership
// using an arbitrary field from an untrusted message.
//
// node.welcome alone does not establish the ownership
// session contract.

func (s *Session) ActivateOwnership(
	ctx context.Context,
	sessionID string,
) error {
	if s == nil {
		return ErrOwnershipSessionUnavailable
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		if s.readErr != nil {
			return s.readErr
		}
		return errSessionClosed
	}

	if s.ownership == nil {
		return ErrOwnershipSessionUnavailable
	}

	// The Panel-issued session ID is bound to this
	// WebSocket during node.welcome. Callers cannot
	// replace it with a different identifier.
	if !validOwnershipSessionID(s.welcome.SessionID) {
		return ErrOwnershipWelcomeSessionRequired
	}

	if sessionID != s.welcome.SessionID {
		return ErrOwnershipSessionIDMismatch
	}

	// Ownership was activated during the trusted
	// node.welcome handshake.
	//
	// Verify that this exact connection still owns
	// the active generation. Never reactivate it.
	return s.ownership.checkActive(ctx)
}
