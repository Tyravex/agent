package commands

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOwnershipSyncGenerationRejectsOldBeginSession(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-new", 11,
	); err != nil {
		t.Fatal(err)
	}

	err := manager.BeginSessionWithGeneration(
		ctx, "session-old", 10,
	)

	if !errors.Is(err, ErrStaleOwnershipGeneration) {
		t.Fatalf("expected stale generation, got %v", err)
	}

	status := manager.Status()

	if status.SessionID != "session-new" ||
		status.Generation != 11 {
		t.Fatalf("new session replaced: %+v", status)
	}
}

func TestOwnershipSyncGenerationRejectsDelayedSnapshot(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-old", 10,
	); err != nil {
		t.Fatal(err)
	}

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-new", 11,
	); err != nil {
		t.Fatal(err)
	}

	err := manager.ApplySnapshotWithGeneration(
		ctx,
		"session-old",
		10,
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 100,
		},
		time.Minute,
	)

	if !errors.Is(err, ErrOwnershipGenerationMismatch) {
		t.Fatalf("expected generation mismatch, got %v", err)
	}

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}
}

func TestOwnershipSyncGenerationRejectsOldDisconnect(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-old", 10,
	); err != nil {
		t.Fatal(err)
	}

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-new", 11,
	); err != nil {
		t.Fatal(err)
	}

	if err := manager.ApplySnapshotWithGeneration(
		ctx,
		"session-new",
		11,
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 1,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	manager.EndSessionWithGeneration("session-old", 10)

	if got := manager.Status().State; got != OwnershipReady {
		t.Fatalf("old disconnect revoked new session: %s", got)
	}
}

func TestOwnershipSyncGenerationPreservesRevision(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-1", 1,
	); err != nil {
		t.Fatal(err)
	}

	if err := manager.ApplySnapshotWithGeneration(
		ctx,
		"session-1",
		1,
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 100,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-2", 2,
	); err != nil {
		t.Fatal(err)
	}

	// A new authenticated generation starts with no active revision.
	// Historical revision metadata must remain intact.
	status := manager.Status()
	if status.State != OwnershipNotReady ||
		status.Revision != 0 ||
		status.HighestRevision != 100 {
		t.Fatalf("unexpected status after reconnect: %+v", status)
	}

	// Panel revisions are scoped to the authenticated session.
	if err := manager.ApplySnapshotWithGeneration(
		ctx,
		"session-2",
		2,
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 1,
		},
		time.Minute,
	); err != nil {
		t.Fatalf("new session revision 1 rejected: %v", err)
	}

	// Replaying the same revision within a session is forbidden.
	err := manager.ApplySnapshotWithGeneration(
		ctx,
		"session-2",
		2,
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 1,
		},
		time.Minute,
	)
	if !errors.Is(err, ErrStaleOwnershipRevision) {
		t.Fatalf("expected stale revision, got %v", err)
	}

	// A previous generation must not modify the active session.
	err = manager.ApplySnapshotWithGeneration(
		ctx,
		"session-1",
		1,
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 101,
		},
		time.Minute,
	)
	if !errors.Is(err, ErrOwnershipGenerationMismatch) {
		t.Fatalf("expected generation mismatch, got %v", err)
	}

	// Higher revisions in the current session are accepted.
	if err := manager.ApplySnapshotWithGeneration(
		ctx,
		"session-2",
		2,
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 2,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	status = manager.Status()
	if status.State != OwnershipReady ||
		status.Revision != 2 ||
		status.HighestRevision != 100 ||
		status.Generation != 2 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestOwnershipSyncGenerationRejectsLegacyTakeover(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-new", 10,
	); err != nil {
		t.Fatal(err)
	}

	err := manager.BeginSession(ctx, "legacy-session")

	if !errors.Is(err, ErrStaleOwnershipGeneration) {
		t.Fatalf("expected stale generation, got %v", err)
	}

	if got := manager.Status().SessionID; got != "session-new" {
		t.Fatalf("legacy session replaced new session: %s", got)
	}
}

func TestOwnershipSyncGenerationRejectsLegacySnapshot(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-new", 10,
	); err != nil {
		t.Fatal(err)
	}

	err := manager.ApplySnapshot(
		ctx,
		"session-new",
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 100,
		},
		time.Minute,
	)

	if !errors.Is(err, ErrOwnershipGenerationMismatch) {
		t.Fatalf("expected generation mismatch, got %v", err)
	}
}

func TestOwnershipSyncGenerationRejectsLegacyDisconnect(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-new", 10,
	); err != nil {
		t.Fatal(err)
	}

	if err := manager.ApplySnapshotWithGeneration(
		ctx,
		"session-new",
		10,
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 1,
		},
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}

	manager.EndSession("session-new")

	if got := manager.Status().State; got != OwnershipReady {
		t.Fatalf("legacy disconnect revoked session: %s", got)
	}
}

func TestOwnershipSyncGenerationConcurrentTakeovers(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	const connections = 32

	var wg sync.WaitGroup

	for i := 1; i <= connections; i++ {
		wg.Add(1)

		go func(generation uint64) {
			defer wg.Done()

			err := manager.BeginSessionWithGeneration(
				ctx,
				"session",
				generation,
			)

			if err != nil &&
				!errors.Is(err, ErrStaleOwnershipGeneration) {
				t.Errorf(
					"generation %d: unexpected error: %v",
					generation,
					err,
				)
			}
		}(uint64(i))
	}

	wg.Wait()

	status := manager.Status()

	if status.Generation != connections {
		t.Fatalf(
			"expected generation %d, got %d",
			connections,
			status.Generation,
		)
	}

	if status.State != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", status.State)
	}
}

func TestOwnershipSyncGenerationRejectsZero(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")

	err := manager.BeginSessionWithGeneration(
		context.Background(),
		"session",
		0,
	)

	if !errors.Is(err, ErrStaleOwnershipGeneration) {
		t.Fatalf("expected stale generation, got %v", err)
	}
}

func TestOwnershipSyncGenerationEndSessionPreservesHighWatermark(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-1", 10,
	); err != nil {
		t.Fatal(err)
	}

	manager.EndSessionWithGeneration("session-1", 10)

	err := manager.BeginSessionWithGeneration(
		ctx, "session-old", 9,
	)

	if !errors.Is(err, ErrStaleOwnershipGeneration) {
		t.Fatalf("expected stale generation, got %v", err)
	}

	if err := manager.BeginSessionWithGeneration(
		ctx, "session-2", 11,
	); err != nil {
		t.Fatal(err)
	}
}

func TestOwnershipSyncGenerationRejectsExcessiveTTL(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx,
		"session-1",
		1,
	); err != nil {
		t.Fatal(err)
	}

	err := manager.ApplySnapshotWithGeneration(
		ctx,
		"session-1",
		1,
		OwnershipSnapshot{
			NodeID:   "node-1",
			Revision: 1,
		},
		6*time.Minute,
	)

	if !errors.Is(err, ErrOwnershipSnapshotExpired) {
		t.Fatalf(
			"expected snapshot expiration error, got %v",
			err,
		)
	}

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}
}
