package transport

import (
	"errors"
	"sync"
)

var (
	ErrDuplicateCommandID = errors.New("duplicate command ID")
	ErrCommandHistoryFull = errors.New("command history limit reached")
)

// commandIDRegistry remembers command IDs across WebSocket
// sessions belonging to the same Client.
//
// Reservations are never released during the lifetime of
// this registry. An operation may have caused an external
// side effect even when its result was not delivered.
type commandIDRegistry struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func newCommandIDRegistry() *commandIDRegistry {
	return &commandIDRegistry{
		seen: make(map[string]struct{}),
	}
}

// reserve atomically records an ID before command execution.
// It never evicts an existing ID.
func (r *commandIDRegistry) reserve(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.seen[id]; exists {
		return ErrDuplicateCommandID
	}

	if len(r.seen) >= maxRememberedCommands {
		return ErrCommandHistoryFull
	}

	r.seen[id] = struct{}{}
	return nil
}
