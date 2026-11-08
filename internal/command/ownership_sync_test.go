package commands

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOwnershipSyncStartsNotReady(t *testing.T) {
	manager := NewOwnershipSyncManager("node-1")

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}

	_, err := manager.Lookup(context.Background(), "service-1")
	if !errors.Is(err, ErrOwnershipNotReady) {
		t.Fatalf("expected not ready, got %v", err)
	}
}

func TestOwnershipSyncRequiresSession(t *testing.T) {
	manager := NewOwnershipSyncManager("node-1")

	err := manager.ApplySnapshot(
		context.Background(),
		"session-1",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 1,
		},
		time.Minute,
	)

	if !errors.Is(err, ErrOwnershipSessionMismatch) {
		t.Fatalf("expected session mismatch, got %v", err)
	}
}

func TestOwnershipSyncReadyAfterSnapshot(t *testing.T) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSession(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	err := manager.ApplySnapshot(
		ctx,
		"session-1",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 1,
			Assignments: []ServiceOwnership{
				{
					ServiceID: "service-1",
					NodeID:    "node-1",
					Enabled:   true,
				},
			},
		},
		time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got := manager.Status().State; got != OwnershipReady {
		t.Fatalf("expected READY, got %s", got)
	}

	ownership, err := manager.Lookup(ctx, "service-1")
	if err != nil {
		t.Fatal(err)
	}

	if !ownership.Enabled {
		t.Fatal("expected enabled ownership")
	}
}

func TestOwnershipSyncExpires(t *testing.T) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	now := time.Date(
		2026, 1, 1, 12, 0, 0, 0, time.UTC,
	)
	manager.now = func() time.Time {
		return now
	}

	if err := manager.BeginSession(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	if err := manager.ApplySnapshot(
		ctx,
		"session-1",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 1,
		},
		60*time.Second,
	); err != nil {
		t.Fatal(err)
	}

	now = now.Add(61 * time.Second)

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected expired NOT_READY, got %s", got)
	}

	_, err := manager.Lookup(ctx, "service-1")
	if !errors.Is(err, ErrOwnershipNotReady) {
		t.Fatalf("expected not ready, got %v", err)
	}
}

func TestOwnershipSyncRejectsStaleRevision(t *testing.T) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSession(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	snapshot := OwnershipSnapshot{
		NodeID:   "node-1",
		Revision: 10,
	}

	if err := manager.ApplySnapshot(
		ctx, "session-1", snapshot, time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	err := manager.ApplySnapshot(
		ctx, "session-1", snapshot, time.Minute,
	)

	if !errors.Is(err, ErrStaleOwnershipRevision) {
		t.Fatalf("expected stale revision, got %v", err)
	}
}

func TestOwnershipSyncRejectsWrongNode(t *testing.T) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSession(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	err := manager.ApplySnapshot(
		ctx,
		"session-1",
		OwnershipSnapshot{
			NodeID:   "node-2",
			Revision: 1,
		},
		time.Minute,
	)

	if !errors.Is(err, ErrInvalidOwnershipSnapshot) {
		t.Fatalf("expected invalid snapshot, got %v", err)
	}
}

func TestOwnershipSyncOldDisconnectDoesNotRevokeNewSession(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSession(ctx, "old"); err != nil {
		t.Fatal(err)
	}

	if err := manager.BeginSession(ctx, "new"); err != nil {
		t.Fatal(err)
	}

	manager.EndSession("old")

	if got := manager.Status().SessionID; got != "new" {
		t.Fatalf("new session was revoked: %q", got)
	}
}

func TestOwnershipSyncEndSessionRevokesAccess(t *testing.T) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSession(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	if err := manager.ApplySnapshot(
		ctx,
		"session-1",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 1,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	manager.EndSession("session-1")

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}

	_, err := manager.Lookup(ctx, "service-1")
	if !errors.Is(err, ErrOwnershipNotReady) {
		t.Fatalf("expected revoked access, got %v", err)
	}
}

func TestOwnershipSyncRejectsInvalidTTL(t *testing.T) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSession(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	err := manager.ApplySnapshot(
		ctx,
		"session-1",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 1,
		},
		0,
	)

	if !errors.Is(err, ErrOwnershipSnapshotExpired) {
		t.Fatalf("expected invalid TTL, got %v", err)
	}
}
