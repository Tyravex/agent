package transport

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrClientClosed = errors.New("transport client is closed")
	ErrClientBusy   = errors.New("transport client has active sessions or connections")
)

type clientLifecycle struct {
	mu         sync.Mutex
	closed     bool
	connecting int
	sessions   map[*Session]struct{}
}

func (c *Client) beginConnect() error {
	c.lifecycle.mu.Lock()
	defer c.lifecycle.mu.Unlock()

	if c.lifecycle.closed {
		return ErrClientClosed
	}

	c.lifecycle.connecting++
	return nil
}

func (c *Client) finishConnect() {
	c.lifecycle.mu.Lock()
	c.lifecycle.connecting--
	c.lifecycle.mu.Unlock()
}

func (c *Client) trackSession(s *Session) {
	c.lifecycle.mu.Lock()

	if c.lifecycle.sessions == nil {
		c.lifecycle.sessions = make(map[*Session]struct{})
	}

	c.lifecycle.sessions[s] = struct{}{}
	c.lifecycle.mu.Unlock()

	go c.releaseSessionWhenDone(s)
}

func (c *Client) releaseSessionWhenDone(s *Session) {
	<-s.Done()

	// Once the read loop has ended, it cannot accept
	// additional commands. An already-running executor
	// must finish before the session is deregistered.
	_ = s.WaitCommands(context.Background())

	c.lifecycle.mu.Lock()
	delete(c.lifecycle.sessions, s)
	c.lifecycle.mu.Unlock()
}

func sessionFullyStopped(s *Session) bool {
	select {
	case <-s.Done():
	default:
		return false
	}

	s.commandState.mu.Lock()
	done := s.commandState.done
	s.commandState.mu.Unlock()

	if done == nil {
		return true
	}

	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (c *Client) closePersistentRegistry() error {
	c.lifecycle.mu.Lock()
	defer c.lifecycle.mu.Unlock()

	if c.lifecycle.closed {
		return nil
	}

	registry, ok := c.commandRegistry.(*persistentCommandRegistry)
	if !ok {
		// Preserve the existing no-op behavior for
		// memory-backed clients.
		return nil
	}

	if c.lifecycle.connecting != 0 {
		return ErrClientBusy
	}

	for session := range c.lifecycle.sessions {
		if !sessionFullyStopped(session) {
			return ErrClientBusy
		}

		// A completed session may still be waiting for
		// its cleanup goroutine to acquire the mutex.
		delete(c.lifecycle.sessions, session)
	}

	// The lifecycle mutex prevents a new connection
	// from starting while the registry is closing.
	if err := registry.Close(); err != nil {
		return err
	}

	c.lifecycle.closed = true
	return nil
}
