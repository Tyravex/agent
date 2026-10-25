package transport

import (
	"context"
	"errors"
	"testing"
)

func TestSendEnvelopeRejectsClosedSession(
	t *testing.T,
) {
	expected := errors.New(
		"session already closed",
	)

	session := &Session{
		closed:  true,
		readErr: expected,
	}

	err := session.sendEnvelope(
		context.Background(),
		"command.accepted",
		CommandAccepted{
			CommandID: "command-123",
		},
	)

	if !errors.Is(err, expected) {
		t.Fatalf(
			"expected closed session error, got %v",
			err,
		)
	}
}

func TestSendEnvelopeRejectsClosedSessionWithoutError(
	t *testing.T,
) {
	session := &Session{
		closed: true,
	}

	err := session.sendEnvelope(
		context.Background(),
		"command.result",
		CommandResult{
			CommandID: "command-123",
			Success:   true,
		},
	)

	if !errors.Is(err, errSessionClosed) {
		t.Fatalf(
			"expected errSessionClosed, got %v",
			err,
		)
	}
}

func TestSendEnvelopeHonorsCancelledContext(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	session := &Session{}

	err := session.sendEnvelope(
		ctx,
		"command.accepted",
		CommandAccepted{
			CommandID: "command-123",
		},
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"expected context.Canceled, got %v",
			err,
		)
	}
}
