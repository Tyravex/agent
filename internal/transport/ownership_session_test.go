package transport

import (
	"context"
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/commands"
)

func TestOwnershipTransportRejectsMissingManager(t *testing.T) {
	_, err := newOwnershipSession(
		context.Background(),
		nil,
	)

	if !errors.Is(err, ErrOwnershipTransportDisabled) {
		t.Fatalf("expected disabled ownership, got %v", err)
	}
}

func TestOwnershipTransportStartsInactive(t *testing.T) {
	ctx := context.Background()

	syncManager := commands.NewOwnershipSyncManager(
		"node-1",
	)

	manager := commands.NewOwnershipConnectionManager(
		syncManager,
	)

	session, err := newOwnershipSession(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()

	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf("expected inactive generation, got %d", got)
	}

	err = session.applySnapshot(
		ctx,
		[]byte(`{"type":"ownership.snapshot"}`),
	)

	if !errors.Is(
		err,
		commands.ErrOwnershipConnectionInactive,
	) {
		t.Fatalf("expected inactive connection, got %v", err)
	}
}

func TestOwnershipTransportActivationIsExplicit(t *testing.T) {
	ctx := context.Background()

	syncManager := commands.NewOwnershipSyncManager(
		"node-1",
	)

	manager := commands.NewOwnershipConnectionManager(
		syncManager,
	)

	session, err := newOwnershipSession(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()

	if err := session.activate(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	if got := manager.ActiveGeneration(); got != 1 {
		t.Fatalf("expected generation 1, got %d", got)
	}

	if status := manager.Status(); status.State != commands.OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", status.State)
	}
}

func TestOwnershipTransportCloseRevokesSession(t *testing.T) {
	ctx := context.Background()

	syncManager := commands.NewOwnershipSyncManager(
		"node-1",
	)

	manager := commands.NewOwnershipConnectionManager(
		syncManager,
	)

	session, err := newOwnershipSession(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}

	if err := session.activate(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	session.close()
	session.close()

	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf("expected no active generation, got %d", got)
	}

	if status := manager.Status(); status.State != commands.OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", status.State)
	}
}

func TestOwnershipTransportOldCloseCannotRevokeNew(t *testing.T) {
	ctx := context.Background()

	syncManager := commands.NewOwnershipSyncManager(
		"node-1",
	)

	manager := commands.NewOwnershipConnectionManager(
		syncManager,
	)

	first, err := newOwnershipSession(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()

	if err := first.activate(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	second, err := newOwnershipSession(ctx, manager)
	if err != nil {
		t.Fatal(err)
	}
	defer second.close()

	if err := second.activate(ctx, "session-2"); err != nil {
		t.Fatal(err)
	}

	first.close()

	if got := manager.ActiveGeneration(); got != 2 {
		t.Fatalf("old close revoked generation 2: got %d", got)
	}
}

func TestOwnershipTransportNilSession(t *testing.T) {
	var session *ownershipSession

	err := session.activate(
		context.Background(),
		"session-1",
	)

	if !errors.Is(err, ErrOwnershipTransportDisabled) {
		t.Fatalf("expected disabled ownership, got %v", err)
	}

	err = session.applySnapshot(
		context.Background(),
		nil,
	)

	if !errors.Is(err, ErrOwnershipTransportDisabled) {
		t.Fatalf("expected disabled ownership, got %v", err)
	}

	session.close()
}
