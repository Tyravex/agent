package commands

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type blockingLifecycleExecutor struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (e *blockingLifecycleExecutor) Start(
	ctx context.Context,
	serviceID string,
) error {
	e.once.Do(func() {
		close(e.started)
	})

	select {
	case <-e.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *blockingLifecycleExecutor) Stop(
	ctx context.Context,
	serviceID string,
) error {
	return nil
}

func (e *blockingLifecycleExecutor) Restart(
	ctx context.Context,
	serviceID string,
) error {
	return nil
}

func newLeasedTestStore(t *testing.T) *MemoryOwnershipStore {
	t.Helper()

	store := NewMemoryOwnershipStore()

	err := store.Upsert(
		context.Background(),
		ServiceOwnership{
			ServiceID: "service-1",
			NodeID:    "node-1",
			Enabled:   true,
		},
	)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	return store
}

func TestLeasedDispatcherAllowsLifecycleOperations(
	t *testing.T,
) {
	tests := []struct {
		operation Operation
		want      string
	}{
		{ServiceStart, "start:service-1"},
		{ServiceStop, "stop:service-1"},
		{ServiceRestart, "restart:service-1"},
	}

	for _, tt := range tests {
		t.Run(string(tt.operation), func(t *testing.T) {
			executor := &fakeExecutor{}
			store := newLeasedTestStore(t)

			dispatcher := NewLeasedDispatcher(
				"node-1",
				store,
				executor,
			)

			err := dispatcher.Execute(
				context.Background(),
				Command{
					ID:        "command-1",
					ServiceID: "service-1",
					Operation: tt.operation,
				},
			)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}

			if len(executor.calls) != 1 ||
				executor.calls[0] != tt.want {
				t.Fatalf(
					"unexpected executor calls: %v",
					executor.calls,
				)
			}
		})
	}
}

func TestLeasedDispatcherRejectsUnauthorizedService(
	t *testing.T,
) {
	store := newLeasedTestStore(t)
	executor := &fakeExecutor{}

	dispatcher := NewLeasedDispatcher(
		"node-2",
		store,
		executor,
	)

	err := dispatcher.Execute(
		context.Background(),
		Command{
			ID:        "command-1",
			ServiceID: "service-1",
			Operation: ServiceStart,
		},
	)

	if !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf("expected denial, got %v", err)
	}

	if len(executor.calls) != 0 {
		t.Fatal("unauthorized command reached executor")
	}
}

func TestLeasedDispatcherRejectsInstallation(
	t *testing.T,
) {
	store := newLeasedTestStore(t)
	executor := &fakeExecutor{}

	dispatcher := NewLeasedDispatcher(
		"node-1",
		store,
		executor,
	)

	err := dispatcher.Execute(
		context.Background(),
		Command{
			ID:        "command-1",
			ServiceID: "service-1",
			Operation: ServiceInstall,
		},
	)

	if !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf("expected installation denial, got %v", err)
	}

	if len(executor.calls) != 0 {
		t.Fatal("installation reached executor")
	}
}

func TestLeasedDispatcherFailsClosedWithoutStore(
	t *testing.T,
) {
	executor := &fakeExecutor{}

	dispatcher := NewLeasedDispatcher(
		"node-1",
		nil,
		executor,
	)

	err := dispatcher.Execute(
		context.Background(),
		Command{
			ID:        "command-1",
			ServiceID: "service-1",
			Operation: ServiceStart,
		},
	)

	if !errors.Is(err, ErrLeaseUnavailable) {
		t.Fatalf("expected unavailable lease, got %v", err)
	}

	if len(executor.calls) != 0 {
		t.Fatal("command reached executor")
	}
}

func TestLeasedDispatcherRevokeWaitsForExecution(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	store := newLeasedTestStore(t)

	executor := &blockingLifecycleExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	dispatcher := NewLeasedDispatcher(
		"node-1",
		store,
		executor,
	)

	command := Command{
		ID:        "command-1",
		ServiceID: "service-1",
		Operation: ServiceStart,
	}

	executionDone := make(chan error, 1)

	go func() {
		executionDone <- dispatcher.Execute(
			ctx,
			command,
		)
	}()

	select {
	case <-executor.started:
		// Execution has entered the leased callback.
	case <-ctx.Done():
		t.Fatal("executor did not start")
	}

	revokeStarted := make(chan struct{})
	revokeDone := make(chan error, 1)

	go func() {
		close(revokeStarted)

		revokeDone <- store.Revoke(
			ctx,
			"service-1",
		)
	}()

	<-revokeStarted

	// Check that Revoke cannot finish while the
	// executor is still inside the leased callback.
	select {
	case err := <-revokeDone:
		t.Fatalf(
			"Revoke finished before execution: %v",
			err,
		)
	case <-time.After(50 * time.Millisecond):
	}

	close(executor.release)

	select {
	case err := <-executionDone:
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("execution did not finish")
	}

	select {
	case err := <-revokeDone:
		if err != nil {
			t.Fatalf("Revoke: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Revoke did not finish")
	}

	// Once Revoke returns, the same service must
	// no longer be authorized for new operations.
	err := dispatcher.Execute(
		context.Background(),
		Command{
			ID:        "command-2",
			ServiceID: "service-1",
			Operation: ServiceStop,
		},
	)

	if !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf(
			"expected denial after revoke, got %v",
			err,
		)
	}
}

func TestLeasedDispatcherReassignmentDeniesOldNode(
	t *testing.T,
) {
	ctx := context.Background()
	store := newLeasedTestStore(t)

	executor := &fakeExecutor{}
	dispatcher := NewLeasedDispatcher(
		"node-1",
		store,
		executor,
	)

	err := store.Upsert(
		ctx,
		ServiceOwnership{
			ServiceID: "service-1",
			NodeID:    "node-2",
			Enabled:   true,
		},
	)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	err = dispatcher.Execute(
		ctx,
		Command{
			ID:        "command-1",
			ServiceID: "service-1",
			Operation: ServiceRestart,
		},
	)

	if !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf(
			"expected denial after reassignment, got %v",
			err,
		)
	}

	if len(executor.calls) != 0 {
		t.Fatal("reassigned service reached executor")
	}
}
