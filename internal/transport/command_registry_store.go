package transport

import (
	"errors"
	"fmt"

	"github.com/Tyravex/agent/internal/config"
)

// commandIDStore reserves command IDs before execution.
//
// Successful reservations are never released because
// execution may have caused an external side effect.
type commandIDStore interface {
	reserve(id string) error
}

// NewClientWithPersistentRegistry creates a Client whose
// command ID history survives reopening the journal.
//
// The journal directory must already exist.
//
// This constructor does not enable command execution.
// An executor must still be explicitly supplied through
// ConnectWithExecutor.
func NewClientWithPersistentRegistry(
	cfg config.Config,
	journalPath string,
) (*Client, error) {
	if journalPath == "" {
		return nil, errors.New(
			"persistent command registry path is empty",
		)
	}

	registry, err := openPersistentCommandRegistry(journalPath)
	if err != nil {
		return nil, fmt.Errorf(
			"initialize persistent command registry: %w",
			err,
		)
	}

	return &Client{
		config:          cfg,
		commandRegistry: registry,
	}, nil
}

// ClosePersistentRegistry closes the persistent journal
// and releases its lock.
//
// It returns ErrClientBusy if a connection is being
// established, a session is still active, or an executor
// has not finished.
//
// After a successful close, the persistent Client cannot
// establish new sessions.
//
// For a memory-backed Client this method is a no-op.
func (c *Client) ClosePersistentRegistry() error {
	return c.closePersistentRegistry()
}
