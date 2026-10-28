package commands

import (
	"context"
	"errors"
	"testing"
)

func authorizedTestCommand(operation Operation) Command {
	return Command{
		ID:        "command-1",
		ServiceID: "service-1",
		Operation: operation,
	}
}

func authorizedTestStore() *fakeOwnershipStore {
	return &fakeOwnershipStore{
		ownership: ServiceOwnership{
			ServiceID: "service-1",
			NodeID:    "node-1",
			Enabled:   true,
		},
	}
}

func TestAuthorizedDispatcherAllowsOwnedLifecycleOperations(
	t *testing.T,
) {
	tests := []struct {
		operation Operation
		wantCall  string
	}{
		{ServiceStart, "start:service-1"},
		{ServiceStop, "stop:service-1"},
		{ServiceRestart, "restart:service-1"},
	}

	for _, tt := range tests {
		t.Run(string(tt.operation), func(t *testing.T) {
			executor := &fakeExecutor{}
			store := authorizedTestStore()

			dispatcher := NewAuthorizedDispatcher(
				executor,
				NewServiceAuthorizer("node-1", store),
			)

			err := dispatcher.Execute(
				context.Background(),
				authorizedTestCommand(tt.operation),
			)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}

			if store.calls != 1 {
				t.Fatalf(
					"expected one ownership lookup, got %d",
					store.calls,
				)
			}

			if len(executor.calls) != 1 ||
				executor.calls[0] != tt.wantCall {
				t.Fatalf(
					"unexpected executor calls: %v",
					executor.calls,
				)
			}
		})
	}
}

func TestAuthorizedDispatcherDenialNeverCallsExecutor(
	t *testing.T,
) {
	tests := []struct {
		name  string
		store *fakeOwnershipStore
	}{
		{
			name: "different node",
			store: &fakeOwnershipStore{
				ownership: ServiceOwnership{
					ServiceID: "service-1",
					NodeID:    "node-2",
					Enabled:   true,
				},
			},
		},
		{
			name: "disabled service",
			store: &fakeOwnershipStore{
				ownership: ServiceOwnership{
					ServiceID: "service-1",
					NodeID:    "node-1",
					Enabled:   false,
				},
			},
		},
		{
			name: "missing service",
			store: &fakeOwnershipStore{
				err: errors.New("service not found"),
			},
		},
		{
			name: "store unavailable",
			store: &fakeOwnershipStore{
				err: errors.New("storage unavailable"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &fakeExecutor{}

			dispatcher := NewAuthorizedDispatcher(
				executor,
				NewServiceAuthorizer("node-1", tt.store),
			)

			err := dispatcher.Execute(
				context.Background(),
				authorizedTestCommand(ServiceStart),
			)

			if !errors.Is(
				err,
				ErrServiceNotAuthorized,
			) {
				t.Fatalf(
					"expected authorization denial, got %v",
					err,
				)
			}

			if len(executor.calls) != 0 {
				t.Fatalf(
					"denied command reached executor: %v",
					executor.calls,
				)
			}
		})
	}
}

func TestAuthorizedDispatcherFailsClosedWithoutAuthorizer(
	t *testing.T,
) {
	executor := &fakeExecutor{}
	dispatcher := NewAuthorizedDispatcher(
		executor,
		nil,
	)

	err := dispatcher.Execute(
		context.Background(),
		authorizedTestCommand(ServiceStart),
	)

	if !errors.Is(
		err,
		ErrAuthorizationUnavailable,
	) {
		t.Fatalf(
			"expected unavailable authorization, got %v",
			err,
		)
	}

	if len(executor.calls) != 0 {
		t.Fatalf(
			"executor was called: %v",
			executor.calls,
		)
	}
}

func TestAuthorizedDispatcherRejectsInstallation(
	t *testing.T,
) {
	executor := &fakeExecutor{}
	store := authorizedTestStore()

	dispatcher := NewAuthorizedDispatcher(
		executor,
		NewServiceAuthorizer("node-1", store),
	)

	err := dispatcher.Execute(
		context.Background(),
		authorizedTestCommand(ServiceInstall),
	)

	if !errors.Is(
		err,
		ErrServiceNotAuthorized,
	) {
		t.Fatalf(
			"expected installation denial, got %v",
			err,
		)
	}

	if store.calls != 0 {
		t.Fatalf(
			"installation caused ownership lookup: %d",
			store.calls,
		)
	}

	if len(executor.calls) != 0 {
		t.Fatal("installation reached lifecycle executor")
	}
}

func TestAuthorizedDispatcherRejectsCancelledContext(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	executor := &fakeExecutor{}
	store := authorizedTestStore()

	dispatcher := NewAuthorizedDispatcher(
		executor,
		NewServiceAuthorizer("node-1", store),
	)

	err := dispatcher.Execute(
		ctx,
		authorizedTestCommand(ServiceRestart),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"expected cancellation, got %v",
			err,
		)
	}

	if store.calls != 0 {
		t.Fatal("cancelled command caused ownership lookup")
	}

	if len(executor.calls) != 0 {
		t.Fatal("cancelled command reached executor")
	}
}

func TestAuthorizedDispatcherRejectsInvalidCommand(
	t *testing.T,
) {
	executor := &fakeExecutor{}
	store := authorizedTestStore()

	dispatcher := NewAuthorizedDispatcher(
		executor,
		NewServiceAuthorizer("node-1", store),
	)

	err := dispatcher.Execute(
		context.Background(),
		Command{
			ID:        "command-1",
			ServiceID: "",
			Operation: ServiceStart,
		},
	)

	if !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf(
			"expected invalid command, got %v",
			err,
		)
	}

	if store.calls != 0 {
		t.Fatal("invalid command caused ownership lookup")
	}

	if len(executor.calls) != 0 {
		t.Fatal("invalid command reached executor")
	}
}

func TestAuthorizedDispatcherPreservesExecutorError(
	t *testing.T,
) {
	expected := errors.New("runtime failed")
	executor := &fakeExecutor{err: expected}
	store := authorizedTestStore()

	dispatcher := NewAuthorizedDispatcher(
		executor,
		NewServiceAuthorizer("node-1", store),
	)

	err := dispatcher.Execute(
		context.Background(),
		authorizedTestCommand(ServiceStop),
	)

	if !errors.Is(err, expected) {
		t.Fatalf(
			"expected runtime error, got %v",
			err,
		)
	}

	if len(executor.calls) != 1 {
		t.Fatalf(
			"expected one executor call, got %v",
			executor.calls,
		)
	}
}

func TestAuthorizedDispatcherDoesNotEnableInstaller(
	t *testing.T,
) {
	executor := &fakeExecutor{}
	installer := &fakeInstaller{}
	store := authorizedTestStore()

	dispatcher := NewAuthorizedDispatcher(
		executor,
		NewServiceAuthorizer("node-1", store),
	)

	// The authorized constructor has no installer parameter.
	// Keep this assertion to detect accidental future wiring.
	if dispatcher.installer != nil {
		t.Fatal("authorized dispatcher unexpectedly has installer")
	}

	if len(installer.calls) != 0 {
		t.Fatal("installer was unexpectedly called")
	}
}
