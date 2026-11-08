package commands

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func ownershipConnectionSnapshot(
	sessionID string,
	revision uint64,
) []byte {
	return []byte(fmt.Sprintf(`{
	"type":"ownership.snapshot",
		"session_id":%q,
		"node_id":"node-1",
		"revision":%d,
		"ttl_seconds":60,
		"assignments":[{
			"service_id":"minecraft-1",
			"node_id":"node-1",
			"enabled":true
		}]
	}`, sessionID, revision))
}

func newOwnershipConnectionTestManager() (
	*OwnershipConnectionManager,
	*OwnershipSyncManager,
) {
	syncManager := NewOwnershipSyncManager("node-1")

	return NewOwnershipConnectionManager(syncManager),
		syncManager
}

func TestOwnershipConnectionStartsNotReady(t *testing.T) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	connection, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if connection.Generation() != 1 {
		t.Fatalf(
			"expected generation 1, got %d",
			connection.Generation(),
		)
	}

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}

	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf("expected no active generation, got %d", got)
	}
}

func TestOwnershipConnectionRejectsSnapshotBeforeActivation(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	connection, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	err = connection.ApplySnapshot(
		ctx,
		ownershipConnectionSnapshot("session-1", 1),
	)

	if !errors.Is(err, ErrOwnershipConnectionInactive) {
		t.Fatalf("expected inactive connection, got %v", err)
	}
}

func TestOwnershipConnectionActivateRequiresFreshSnapshot(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	connection, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := connection.Activate(
		ctx,
		"session-1",
	); err != nil {
		t.Fatal(err)
	}

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}

	if got := manager.ActiveGeneration(); got != 1 {
		t.Fatalf("expected generation 1, got %d", got)
	}
}

func TestOwnershipConnectionAcceptsValidSnapshot(
	t *testing.T,
) {
	manager, syncManager := newOwnershipConnectionTestManager()
	ctx := context.Background()

	connection, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := connection.Activate(
		ctx,
		"session-1",
	); err != nil {
		t.Fatal(err)
	}

	if err := connection.ApplySnapshot(
		ctx,
		ownershipConnectionSnapshot("session-1", 10),
	); err != nil {
		t.Fatal(err)
	}

	status := manager.Status()

	if status.State != OwnershipReady ||
		status.Revision != 10 ||
		status.Generation != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}

	ownership, err := syncManager.Lookup(
		ctx,
		"minecraft-1",
	)
	if err != nil {
		t.Fatal(err)
	}

	if !ownership.Enabled {
		t.Fatal("service should be enabled")
	}
}

func TestOwnershipConnectionRejectsWrongSession(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	connection, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := connection.Activate(
		ctx,
		"session-1",
	); err != nil {
		t.Fatal(err)
	}

	err = connection.ApplySnapshot(
		ctx,
		ownershipConnectionSnapshot("session-2", 10),
	)

	if !errors.Is(err, ErrOwnershipSessionMismatch) {
		t.Fatalf("expected session mismatch, got %v", err)
	}

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}
}

func TestOwnershipConnectionReconnectResetsAuthorization(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	first, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := first.Activate(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	if err := first.ApplySnapshot(
		ctx,
		ownershipConnectionSnapshot("session-1", 10),
	); err != nil {
		t.Fatal(err)
	}

	second, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := second.Activate(ctx, "session-2"); err != nil {
		t.Fatal(err)
	}

	status := manager.Status()

	if status.State != OwnershipNotReady ||
		status.Generation != 2 ||
		status.HighestRevision != 10 {
		t.Fatalf("unexpected status: %+v", status)
	}

	if err := second.ApplySnapshot(
		ctx,
		ownershipConnectionSnapshot("session-2", 11),
	); err != nil {
		t.Fatal(err)
	}

	if got := manager.Status().State; got != OwnershipReady {
		t.Fatalf("expected READY, got %s", got)
	}
}

func TestOwnershipConnectionRejectsDelayedSnapshot(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	first, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := first.Activate(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	second, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := second.Activate(ctx, "session-2"); err != nil {
		t.Fatal(err)
	}

	err = first.ApplySnapshot(
		ctx,
		ownershipConnectionSnapshot("session-1", 100),
	)

	if !errors.Is(err, ErrOwnershipConnectionInactive) &&
		!errors.Is(err, ErrOwnershipConnectionSuperseded) {
		t.Fatalf("expected old connection rejection, got %v", err)
	}

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}
}

func TestOwnershipConnectionOldDisconnectCannotRevokeNew(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	first, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := first.Activate(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	second, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := second.Activate(ctx, "session-2"); err != nil {
		t.Fatal(err)
	}

	if err := second.ApplySnapshot(
		ctx,
		ownershipConnectionSnapshot("session-2", 20),
	); err != nil {
		t.Fatal(err)
	}

	first.Close()

	status := manager.Status()

	if status.State != OwnershipReady ||
		status.Generation != 2 ||
		status.Revision != 20 {
		t.Fatalf("old disconnect revoked new session: %+v", status)
	}
}

func TestOwnershipConnectionCurrentDisconnectRevokes(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	connection, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := connection.Activate(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	if err := connection.ApplySnapshot(
		ctx,
		ownershipConnectionSnapshot("session-1", 10),
	); err != nil {
		t.Fatal(err)
	}

	connection.Close()

	status := manager.Status()

	if status.State != OwnershipNotReady ||
		status.HighestRevision != 10 {
		t.Fatalf("unexpected status after close: %+v", status)
	}

	if got := manager.ActiveGeneration(); got != 0 {
		t.Fatalf("expected no active generation, got %d", got)
	}
}

func TestOwnershipConnectionClosedCannotReactivate(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	connection, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	connection.Close()

	err = connection.Activate(ctx, "session-1")

	if !errors.Is(err, ErrOwnershipConnectionClosed) {
		t.Fatalf("expected closed connection, got %v", err)
	}
}

func TestOwnershipConnectionOlderCannotTakeOver(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	first, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	second, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := second.Activate(ctx, "session-2"); err != nil {
		t.Fatal(err)
	}

	err = first.Activate(ctx, "session-1")

	if !errors.Is(err, ErrOwnershipConnectionSuperseded) &&
		!errors.Is(err, ErrStaleOwnershipGeneration) {
		t.Fatalf("expected stale connection, got %v", err)
	}
}

func TestOwnershipConnectionInvalidMessagePreservesState(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	connection, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := connection.Activate(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	if err := connection.ApplySnapshot(
		ctx,
		ownershipConnectionSnapshot("session-1", 10),
	); err != nil {
		t.Fatal(err)
	}

	before := manager.Status()

	invalid := []byte(`{
		"type":"ownership.snapshot",
		"session_id":"session-1",
		"node_id":"node-1",
		"revision":999,
		"ttl_seconds":60,
		"assignments":[],
		"generation":999
	}`)

	err = connection.ApplySnapshot(ctx, invalid)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}

	after := manager.Status()

	if after.State != before.State ||
		after.Revision != before.Revision ||
		after.HighestRevision != before.HighestRevision ||
		after.Generation != before.Generation ||
		!after.ExpiresAt.Equal(before.ExpiresAt) {
		t.Fatalf(
			"invalid message changed state: before=%+v after=%+v",
			before,
			after,
		)
	}
}

func TestOwnershipConnectionConcurrentOpen(
	t *testing.T,
) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	const count = 64

	var wg sync.WaitGroup
	generations := make(chan uint64, count)
	errorsCh := make(chan error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			connection, err := manager.Open(ctx)
			if err != nil {
				errorsCh <- err
				return
			}

			generations <- connection.Generation()
		}()
	}

	wg.Wait()
	close(generations)
	close(errorsCh)

	for err := range errorsCh {
		t.Fatal(err)
	}

	seen := make(map[uint64]struct{})

	for generation := range generations {
		if generation == 0 {
			t.Fatal("zero generation assigned")
		}

		if _, exists := seen[generation]; exists {
			t.Fatalf("duplicate generation %d", generation)
		}

		seen[generation] = struct{}{}
	}

	if len(seen) != count {
		t.Fatalf("expected %d generations, got %d", count, len(seen))
	}
}

func TestOwnershipConnectionNilManager(t *testing.T) {
	var manager *OwnershipConnectionManager

	_, err := manager.Open(context.Background())

	if !errors.Is(err, ErrOwnershipConnectionUnavailable) {
		t.Fatalf("expected unavailable manager, got %v", err)
	}

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}
}

func TestOwnershipConnectionCanceledContext(t *testing.T) {
	manager, _ := newOwnershipConnectionTestManager()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := manager.Open(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestOwnershipConnectionCloseIsIdempotent(t *testing.T) {
	manager, _ := newOwnershipConnectionTestManager()
	ctx := context.Background()

	connection, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := connection.Activate(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}

	connection.Close()
	connection.Close()

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}
}
