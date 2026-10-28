package commands

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func readyOwnershipSyncManager(
	t *testing.T,
) *OwnershipSyncManager {
	t.Helper()

	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSession(
		ctx,
		"session-1",
	); err != nil {
		t.Fatal(err)
	}

	if err := manager.ApplySnapshot(
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
	); err != nil {
		t.Fatal(err)
	}

	return manager
}

func TestOwnershipSyncLeaseAllowsAuthorizedOperation(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	var executed bool

	err := manager.WithAuthorizedService(
		context.Background(),
		"node-1",
		"service-1",
		func() error {
			executed = true
			return nil
		},
	)

	if err != nil {
		t.Fatal(err)
	}

	if !executed {
		t.Fatal("authorized callback was not executed")
	}
}

func TestOwnershipSyncLeaseRejectsWrongNode(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	var executed bool

	err := manager.WithAuthorizedService(
		context.Background(),
		"node-2",
		"service-1",
		func() error {
			executed = true
			return nil
		},
	)

	if !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}

	if executed {
		t.Fatal("unauthorized callback executed")
	}
}

func TestOwnershipSyncLeaseRejectsUnknownService(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	var executed bool

	err := manager.WithAuthorizedService(
		context.Background(),
		"node-1",
		"service-unknown",
		func() error {
			executed = true
			return nil
		},
	)

	if !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}

	if executed {
		t.Fatal("unknown service callback executed")
	}
}

func TestOwnershipSyncLeaseRejectsExpiredSnapshot(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	manager.mu.Lock()
	manager.expiresAt = time.Now().Add(-time.Second)
	manager.mu.Unlock()

	var executed bool

	err := manager.WithAuthorizedService(
		context.Background(),
		"node-1",
		"service-1",
		func() error {
			executed = true
			return nil
		},
	)

	if !errors.Is(err, ErrOwnershipNotReady) {
		t.Fatalf("expected not ready, got %v", err)
	}

	if executed {
		t.Fatal("expired authorization executed")
	}
}

func TestOwnershipSyncLeaseRejectsRevokedService(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)
	ctx := context.Background()

	err := manager.ApplySnapshot(
		ctx,
		"session-1",
		OwnershipSnapshot{
			NodeID:      "node-1",
			Revision:    2,
			Assignments: nil,
		},
		time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}

	var executed bool

	err = manager.WithAuthorizedService(
		ctx,
		"node-1",
		"service-1",
		func() error {
			executed = true
			return nil
		},
	)

	if !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}

	if executed {
		t.Fatal("revoked service callback executed")
	}
}

func TestOwnershipSyncLeaseAllowsSessionEndDuringOperation(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)

	go func() {
		finished <- manager.WithAuthorizedService(
			context.Background(),
			"node-1",
			"service-1",
			func() error {
				close(started)
				<-release
				return nil
			},
		)
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not start")
	}

	ended := make(chan struct{})

	go func() {
		manager.EndSession("session-1")
		close(ended)
	}()

	select {
	case <-ended:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("session invalidation blocked by operation")
	}

	close(release)

	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("operation failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not finish")
	}

	_, err := manager.Lookup(
		context.Background(),
		"service-1",
	)

	if !errors.Is(err, ErrOwnershipNotReady) {
		t.Fatalf("expected revoked access, got %v", err)
	}
}

func TestOwnershipSyncLeaseAllowsSnapshotUpdateDuringOperation(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)

	go func() {
		finished <- manager.WithAuthorizedService(
			context.Background(),
			"node-1",
			"service-1",
			func() error {
				close(started)
				<-release
				return nil
			},
		)
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not start")
	}

	updated := make(chan error, 1)

	go func() {
		updated <- manager.ApplySnapshot(
			context.Background(),
			"session-1",
			OwnershipSnapshot{
				NodeID:      "node-1",
				Revision:    2,
				Assignments: nil,
			},
			time.Minute,
		)
	}()

	select {
	case err := <-updated:
		if err != nil {
			close(release)
			t.Fatalf("snapshot update failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("snapshot update blocked by operation")
	}

	close(release)

	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("operation failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("operation did not finish")
	}

	err := manager.WithAuthorizedService(
		context.Background(),
		"node-1",
		"service-1",
		func() error {
			t.Fatal("revoked service executed")
			return nil
		},
	)

	if !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestOwnershipSyncLeasePropagatesCallbackError(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	expected := errors.New("executor failed")

	err := manager.WithAuthorizedService(
		context.Background(),
		"node-1",
		"service-1",
		func() error {
			return expected
		},
	)

	if !errors.Is(err, expected) {
		t.Fatalf("expected callback error, got %v", err)
	}
}

func TestOwnershipSyncLeaseConcurrentOperations(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	const operations = 32

	var wg sync.WaitGroup
	var executed atomic.Int32

	for i := 0; i < operations; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			err := manager.WithAuthorizedService(
				context.Background(),
				"node-1",
				"service-1",
				func() error {
					executed.Add(1)
					return nil
				},
			)

			if err != nil {
				t.Errorf("operation failed: %v", err)
			}
		}()
	}

	wg.Wait()

	if got := executed.Load(); got != operations {
		t.Fatalf(
			"executed %d operations, want %d",
			got,
			operations,
		)
	}
}

func TestOwnershipSyncLeaseRejectsCanceledContext(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	var executed bool

	err := manager.WithAuthorizedService(
		ctx,
		"node-1",
		"service-1",
		func() error {
			executed = true
			return nil
		},
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context, got %v", err)
	}

	if executed {
		t.Fatal("canceled operation executed")
	}
}

func TestOwnershipSyncLeaseRejectsNilCallback(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	err := manager.WithAuthorizedService(
		context.Background(),
		"node-1",
		"service-1",
		nil,
	)

	if !errors.Is(err, ErrLeaseUnavailable) {
		t.Fatalf("expected lease unavailable, got %v", err)
	}
}

func TestOwnershipSyncLeaseDispatcherRejectsRevokedService(
	t *testing.T,
) {
	manager := readyOwnershipSyncManager(t)

	var started atomic.Int32

	executor := &ownershipSyncTestExecutor{
		started: &started,
	}

	dispatcher := NewLeasedDispatcher(
		"node-1",
		manager,
		executor,
	)

	ctx := context.Background()

	command := Command{
		ID:        "test-command-1",
		Operation: ServiceStart,
		ServiceID: "service-1",
	}

	if err := dispatcher.Execute(ctx, command); err != nil {
		t.Fatalf("authorized execution failed: %v", err)
	}

	if got := started.Load(); got != 1 {
		t.Fatalf("expected 1 execution, got %d", got)
	}

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

	err := dispatcher.Execute(ctx, command)

	if !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}

	if got := started.Load(); got != 1 {
		t.Fatalf(
			"revoked command executed, count=%d",
			got,
		)
	}
}

type ownershipSyncTestExecutor struct {
	started *atomic.Int32
}

func (e *ownershipSyncTestExecutor) Start(
	ctx context.Context,
	serviceID string,
) error {
	e.started.Add(1)
	return nil
}

func (e *ownershipSyncTestExecutor) Stop(
	ctx context.Context,
	serviceID string,
) error {
	return nil
}

func (e *ownershipSyncTestExecutor) Restart(
	ctx context.Context,
	serviceID string,
) error {
	return nil
}
