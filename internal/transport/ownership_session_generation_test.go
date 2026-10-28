package transport

import (
	"context"
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/commands"
)

func TestOwnershipSessionBindingSupersededConnection(t *testing.T) {
	const (
		firstID  = "session-0000000000000001"
		secondID = "session-0000000000000002"
	)

	ctx := context.Background()
	manager := ownershipIntegrationManager()

	firstOwnership, err := newOwnershipSession(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer firstOwnership.close()

	if err := firstOwnership.activate(ctx, firstID); err != nil {
		t.Fatalf("activate first: %v", err)
	}

	first := &Session{
		welcome: Welcome{
			NodeID:    "node-1",
			SessionID: firstID,
		},
		ownership: firstOwnership,
	}

	if err := first.ActivateOwnership(ctx, firstID); err != nil {
		t.Fatalf("first connection should be active: %v", err)
	}

	if got := manager.ActiveGeneration(); got != 1 {
		t.Fatalf("first generation = %d, want 1", got)
	}

	secondOwnership, err := newOwnershipSession(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer secondOwnership.close()

	if err := secondOwnership.activate(ctx, secondID); err != nil {
		t.Fatalf("activate second: %v", err)
	}

	second := &Session{
		welcome: Welcome{
			NodeID:    "node-1",
			SessionID: secondID,
		},
		ownership: secondOwnership,
	}

	if got := manager.ActiveGeneration(); got != 2 {
		t.Fatalf("second generation = %d, want 2", got)
	}

	err = first.ActivateOwnership(ctx, firstID)
	if !errors.Is(err, commands.ErrOwnershipConnectionSuperseded) {
		t.Fatalf(
			"expected superseded connection, got %v",
			err,
		)
	}

	if err := second.ActivateOwnership(ctx, secondID); err != nil {
		t.Fatalf("second connection should be active: %v", err)
	}

	if got := manager.ActiveGeneration(); got != 2 {
		t.Fatalf(
			"old connection changed active generation: %d",
			got,
		)
	}

	firstOwnership.close()

	if got := manager.ActiveGeneration(); got != 2 {
		t.Fatalf(
			"closing old connection revoked new generation: %d",
			got,
		)
	}

	if err := second.ActivateOwnership(ctx, secondID); err != nil {
		t.Fatalf(
			"second connection lost ownership after old close: %v",
			err,
		)
	}

	secondOwnership.close()

	err = second.ActivateOwnership(ctx, secondID)
	if !errors.Is(err, commands.ErrOwnershipConnectionClosed) {
		t.Fatalf(
			"expected closed connection error, got %v",
			err,
		)
	}

	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf(
			"closed connection retained generation %d",
			got,
		)
	}
}

func TestOwnershipSessionBindingInactiveConnection(t *testing.T) {
	ctx := context.Background()
	manager := ownershipIntegrationManager()

	ownership, err := newOwnershipSession(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer ownership.close()

	session := &Session{
		welcome: Welcome{
			NodeID:    "node-1",
			SessionID: "session-0000000000000001",
		},
		ownership: ownership,
	}

	err = session.ActivateOwnership(
		ctx,
		session.welcome.SessionID,
	)

	if !errors.Is(err, commands.ErrOwnershipConnectionInactive) {
		t.Fatalf(
			"expected inactive connection, got %v",
			err,
		)
	}

	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf(
			"inactive connection changed generation: %d",
			got,
		)
	}
}
