package transport

import (
	"context"
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/commands"
)

func TestOwnershipSessionBindingRejectsReplacement(t *testing.T) {
	const sessionID = "session-0000000000000001"

	manager := ownershipIntegrationManager()

	ownership, err := newOwnershipSession(
		context.Background(),
		manager,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer ownership.close()

	if err := ownership.activate(
		context.Background(),
		sessionID,
	); err != nil {
		t.Fatal(err)
	}

	session := &Session{
		welcome: Welcome{
			NodeID:    "node-1",
			SessionID: sessionID,
		},
		ownership: ownership,
	}

	generationBefore := manager.ActiveGeneration()

	err = session.ActivateOwnership(
		context.Background(),
		"session-0000000000000002",
	)

	if !errors.Is(err, ErrOwnershipSessionIDMismatch) {
		t.Fatalf(
			"expected session ID mismatch, got %v",
			err,
		)
	}

	if got := manager.ActiveGeneration(); got != generationBefore {
		t.Fatalf(
			"session replacement changed generation: %d -> %d",
			generationBefore,
			got,
		)
	}

	if got := manager.Status().State; got != commands.OwnershipNotReady {
		t.Fatalf(
			"session replacement changed ownership state: %s",
			got,
		)
	}
}

func TestOwnershipSessionBindingSameIDIsNoOp(t *testing.T) {
	const sessionID = "session-0000000000000001"

	manager := ownershipIntegrationManager()

	ownership, err := newOwnershipSession(
		context.Background(),
		manager,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer ownership.close()

	if err := ownership.activate(
		context.Background(),
		sessionID,
	); err != nil {
		t.Fatal(err)
	}

	session := &Session{
		welcome: Welcome{
			NodeID:    "node-1",
			SessionID: sessionID,
		},
		ownership: ownership,
	}

	generationBefore := manager.ActiveGeneration()

	for i := 0; i < 10; i++ {
		if err := session.ActivateOwnership(
			context.Background(),
			sessionID,
		); err != nil {
			t.Fatalf(
				"repeated activation %d: %v",
				i,
				err,
			)
		}
	}

	if got := manager.ActiveGeneration(); got != generationBefore {
		t.Fatalf(
			"repeated activation changed generation: %d -> %d",
			generationBefore,
			got,
		)
	}
}

func TestOwnershipSessionBindingRejectsMissingWelcomeID(t *testing.T) {
	manager := ownershipIntegrationManager()

	ownership, err := newOwnershipSession(
		context.Background(),
		manager,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer ownership.close()

	session := &Session{
		ownership: ownership,
	}

	err = session.ActivateOwnership(
		context.Background(),
		"session-0000000000000001",
	)

	if !errors.Is(err, ErrOwnershipWelcomeSessionRequired) {
		t.Fatalf(
			"expected missing welcome session ID, got %v",
			err,
		)
	}

	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf(
			"missing welcome activated generation %d",
			got,
		)
	}
}
