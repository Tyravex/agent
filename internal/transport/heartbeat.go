package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const heartbeatTimeout = 10 * time.Second

type DockerStatus struct {
	Available bool    `json:"available"`
	Version   *string `json:"version"`
}

type Heartbeat struct {
	UptimeSeconds int64        `json:"uptimeSeconds"`
	Docker        DockerStatus `json:"docker"`
}

func (s *Session) SendHeartbeat(
	ctx context.Context,
	heartbeat Heartbeat,
) error {
	const maxSafeInteger = int64(
		9007199254740991,
	)

	if heartbeat.UptimeSeconds < 0 ||
		heartbeat.UptimeSeconds > maxSafeInteger {
		return errors.New(
			"uptimeSeconds must be a non-negative safe integer",
		)
	}

	if heartbeat.Docker.Version != nil {
		value := *heartbeat.Docker.Version

		if strings.TrimSpace(value) == "" ||
			len(value) > 128 {
			return errors.New(
				"invalid Docker version",
			)
		}
	}

	ctx, cancel := context.WithTimeout(
		ctx,
		heartbeatTimeout,
	)
	defer cancel()

	ack := make(chan error, 1)

	s.mu.Lock()

	if s.closed {
		err := s.readErr
		s.mu.Unlock()

		if err != nil {
			return err
		}
		return errSessionClosed
	}

	if s.pending != nil {
		s.mu.Unlock()
		return errors.New(
			"heartbeat already pending",
		)
	}

	s.pending = ack
	s.mu.Unlock()

	// All writes after handshake go through sendEnvelope.
	// This prevents heartbeat and command replies from
	// writing concurrently to the same WebSocket session.
	if err := s.sendEnvelope(
		ctx,
		"node.heartbeat",
		heartbeat,
	); err != nil {
		wrapped := fmt.Errorf(
			"send node.heartbeat: %w",
			err,
		)

		s.shutdown(wrapped)
		return wrapped
	}

	select {
	case err := <-ack:
		return err

	case <-ctx.Done():
		wrapped := fmt.Errorf(
			"heartbeat timeout or cancellation: %w",
			ctx.Err(),
		)

		s.shutdown(wrapped)
		return wrapped

	case <-s.done:
		select {
		case err := <-ack:
			return err

		default:
			if err := s.Err(); err != nil {
				return err
			}
			return errSessionClosed
		}
	}
}

func validateHeartbeatACK(
	data json.RawMessage,
) error {
	var payload map[string]json.RawMessage

	if err := json.Unmarshal(
		data,
		&payload,
	); err != nil {
		return fmt.Errorf(
			"invalid heartbeat ACK payload: %w",
			err,
		)
	}

	if payload == nil ||
		len(payload) != 0 {
		return errors.New(
			"heartbeat ACK payload must be an empty object",
		)
	}

	return nil
}
