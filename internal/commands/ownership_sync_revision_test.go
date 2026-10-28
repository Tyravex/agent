package commands

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOwnershipSyncRevisionSurvivesReconnect(t *testing.T) {
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
			Revision: 100,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	manager.EndSession("session-1")

	status := manager.Status()

	if status.State != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", status.State)
	}

	if status.Revision != 0 {
		t.Fatalf("expected inactive revision 0, got %d", status.Revision)
	}

	if status.HighestRevision != 100 {
		t.Fatalf(
			"expected highest revision 100, got %d",
			status.HighestRevision,
		)
	}

	if err := manager.BeginSession(ctx, "session-2"); err != nil {
		t.Fatal(err)
	}

	for _, revision := range []uint64{50, 100} {
		err := manager.ApplySnapshot(
			ctx,
			"session-2",
			OwnershipSnapshot{
				NodeID:   "node-1",
				Revision: revision,
			},
			time.Minute,
		)

		if !errors.Is(err, ErrStaleOwnershipRevision) {
			t.Fatalf(
				"revision %d: expected stale revision, got %v",
				revision,
				err,
			)
		}
	}

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}

	if err := manager.ApplySnapshot(
		ctx,
		"session-2",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 101,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	status = manager.Status()

	if status.State != OwnershipReady {
		t.Fatalf("expected READY, got %s", status.State)
	}

	if status.Revision != 101 || status.HighestRevision != 101 {
		t.Fatalf(
			"unexpected revisions: active=%d highest=%d",
			status.Revision,
			status.HighestRevision,
		)
	}
}

func TestOwnershipSyncRevisionRejectsDelayedOldSession(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSession(ctx, "old-session"); err != nil {
		t.Fatal(err)
	}

	if err := manager.ApplySnapshot(
		ctx,
		"old-session",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 10,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	if err := manager.BeginSession(ctx, "new-session"); err != nil {
		t.Fatal(err)
	}

	err := manager.ApplySnapshot(
		ctx,
		"old-session",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 999,
		},
		time.Minute,
	)

	if !errors.Is(err, ErrOwnershipSessionMismatch) {
		t.Fatalf("expected session mismatch, got %v", err)
	}

	if got := manager.Status().HighestRevision; got != 10 {
		t.Fatalf("old session changed highest revision: %d", got)
	}

	if err := manager.ApplySnapshot(
		ctx,
		"new-session",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 11,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}
}

func TestOwnershipSyncRevisionRejectsRollbackAfterExpiration(
	t *testing.T,
) {
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
			Revision: 20,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	now = now.Add(2 * time.Minute)

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}

	err := manager.ApplySnapshot(
		ctx,
		"session-1",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 19,
		},
		time.Minute,
	)

	if !errors.Is(err, ErrStaleOwnershipRevision) {
		t.Fatalf("expected stale revision, got %v", err)
	}

	if err := manager.ApplySnapshot(
		ctx,
		"session-1",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 21,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	if got := manager.Status().State; got != OwnershipReady {
		t.Fatalf("expected READY, got %s", got)
	}
}

func TestOwnershipSyncRevisionRejectsRollbackAfterRevocation(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)
	ctx := context.Background()

	if err := manager.ApplySnapshot(
		ctx,
		"session-1",
		OwnershipSnapshot{
			NodeID:      "node-1",
			Revision:    2,
			Assignments: nil,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	manager.EndSession("session-1")

	if err := manager.BeginSession(ctx, "session-2"); err != nil {
		t.Fatal(err)
	}

	err := manager.ApplySnapshot(
		ctx,
		"session-2",
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

	if !errors.Is(err, ErrStaleOwnershipRevision) {
		t.Fatalf("expected stale revision, got %v", err)
	}

	_, err = manager.Lookup(ctx, "service-1")

	if !errors.Is(err, ErrOwnershipNotReady) {
		t.Fatalf("service unexpectedly authorized: %v", err)
	}
}

func TestOwnershipSyncRevisionConcurrentSnapshots(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSession(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	const snapshots = 32

	var wg sync.WaitGroup

	for i := 1; i <= snapshots; i++ {
		wg.Add(1)

		go func(revision uint64) {
			defer wg.Done()

			err := manager.ApplySnapshot(
				ctx,
				"session-1",
				OwnershipSnapshot{
					NodeID:   "node-1",
					Revision: revision,
				},
				time.Minute,
			)

			if err != nil &&
				!errors.Is(err, ErrStaleOwnershipRevision) {
				t.Errorf(
					"revision %d: unexpected error: %v",
					revision,
					err,
				)
			}
		}(uint64(i))
	}

	wg.Wait()

	status := manager.Status()

	if status.Revision != snapshots {
		t.Fatalf(
			"expected active revision %d, got %d",
			snapshots,
			status.Revision,
		)
	}

	if status.HighestRevision != snapshots {
		t.Fatalf(
			"expected highest revision %d, got %d",
			snapshots,
			status.HighestRevision,
		)
	}
}

func TestOwnershipSyncRevisionNewManagerStartsWithoutHistory(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")

	status := manager.Status()

	if status.Revision != 0 || status.HighestRevision != 0 {
		t.Fatalf(
			"unexpected initial revisions: active=%d highest=%d",
			status.Revision,
			status.HighestRevision,
		)
	}

	if status.State != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", status.State)
	}
}
