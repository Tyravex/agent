package commands

import (
	"context"
	"errors"
	"testing"
)

type fakeOwnershipStore struct {
	ownership ServiceOwnership
	err       error
	calls     int
}

func (f *fakeOwnershipStore) Lookup(
	ctx context.Context,
	serviceID string,
) (ServiceOwnership, error) {
	f.calls++

	if f.err != nil {
		return ServiceOwnership{}, f.err
	}

	return f.ownership, nil
}

func TestServiceAuthorizerAllowsOwnedLifecycleCommands(
	t *testing.T,
) {
	operations := []Operation{
		ServiceStart,
		ServiceStop,
		ServiceRestart,
	}

	for _, operation := range operations {
		t.Run(string(operation), func(t *testing.T) {
			store := &fakeOwnershipStore{
				ownership: ServiceOwnership{
					ServiceID: "service-1",
					NodeID:    "node-1",
					Enabled:   true,
				},
			}

			authorizer := NewServiceAuthorizer(
				"node-1",
				store,
			)

			err := authorizer.Authorize(
				context.Background(),
				Command{
					ID:        "command-1",
					ServiceID: "service-1",
					Operation: operation,
				},
			)
			if err != nil {
				t.Fatalf(
					"expected authorization, got %v",
					err,
				)
			}

			if store.calls != 1 {
				t.Fatalf(
					"expected one lookup, got %d",
					store.calls,
				)
			}
		})
	}
}

func TestServiceAuthorizerRejectsUnauthorizedCommands(
	t *testing.T,
) {
	tests := []struct {
		name      string
		nodeID    string
		store     *fakeOwnershipStore
		command   Command
		wantCalls int
	}{
		{
			name:   "different node",
			nodeID: "node-1",
			store: &fakeOwnershipStore{
				ownership: ServiceOwnership{
					ServiceID: "service-1",
					NodeID:    "node-2",
					Enabled:   true,
				},
			},
			command: Command{
				ID:        "command-1",
				ServiceID: "service-1",
				Operation: ServiceStart,
			},
			wantCalls: 1,
		},
		{
			name:   "disabled service",
			nodeID: "node-1",
			store: &fakeOwnershipStore{
				ownership: ServiceOwnership{
					ServiceID: "service-1",
					NodeID:    "node-1",
					Enabled:   false,
				},
			},
			command: Command{
				ID:        "command-1",
				ServiceID: "service-1",
				Operation: ServiceStop,
			},
			wantCalls: 1,
		},
		{
			name:   "mismatched service ID",
			nodeID: "node-1",
			store: &fakeOwnershipStore{
				ownership: ServiceOwnership{
					ServiceID: "service-2",
					NodeID:    "node-1",
					Enabled:   true,
				},
			},
			command: Command{
				ID:        "command-1",
				ServiceID: "service-1",
				Operation: ServiceRestart,
			},
			wantCalls: 1,
		},
		{
			name:   "lookup failure",
			nodeID: "node-1",
			store: &fakeOwnershipStore{
				err: errors.New("database unavailable"),
			},
			command: Command{
				ID:        "command-1",
				ServiceID: "service-1",
				Operation: ServiceStart,
			},
			wantCalls: 1,
		},
		{
			name:   "installation forbidden",
			nodeID: "node-1",
			store: &fakeOwnershipStore{
				ownership: ServiceOwnership{
					ServiceID: "service-1",
					NodeID:    "node-1",
					Enabled:   true,
				},
			},
			command: Command{
				ID:        "command-1",
				ServiceID: "service-1",
				Operation: ServiceInstall,
			},
			wantCalls: 0,
		},
		{
			name:   "invalid service ID",
			nodeID: "node-1",
			store:  &fakeOwnershipStore{},
			command: Command{
				ID:        "command-1",
				ServiceID: "../other",
				Operation: ServiceStart,
			},
			wantCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := NewServiceAuthorizer(
				tt.nodeID,
				tt.store,
			)

			err := authorizer.Authorize(
				context.Background(),
				tt.command,
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

			if tt.store.calls != tt.wantCalls {
				t.Fatalf(
					"expected %d lookups, got %d",
					tt.wantCalls,
					tt.store.calls,
				)
			}
		})
	}
}

func TestServiceAuthorizerFailsClosedWithoutConfiguration(
	t *testing.T,
) {
	command := Command{
		ID:        "command-1",
		ServiceID: "service-1",
		Operation: ServiceStart,
	}

	tests := []struct {
		name       string
		authorizer *ServiceAuthorizer
	}{
		{
			name:       "nil authorizer",
			authorizer: nil,
		},
		{
			name: "missing node ID",
			authorizer: NewServiceAuthorizer(
				"",
				&fakeOwnershipStore{},
			),
		},
		{
			name: "missing store",
			authorizer: NewServiceAuthorizer(
				"node-1",
				nil,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.authorizer.Authorize(
				context.Background(),
				command,
			)

			if !errors.Is(
				err,
				ErrAuthorizationUnavailable,
			) {
				t.Fatalf(
					"expected unavailable, got %v",
					err,
				)
			}
		})
	}
}

func TestServiceAuthorizerHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	store := &fakeOwnershipStore{
		ownership: ServiceOwnership{
			ServiceID: "service-1",
			NodeID:    "node-1",
			Enabled:   true,
		},
	}

	authorizer := NewServiceAuthorizer(
		"node-1",
		store,
	)

	err := authorizer.Authorize(
		ctx,
		Command{
			ID:        "command-1",
			ServiceID: "service-1",
			Operation: ServiceStart,
		},
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"expected cancellation, got %v",
			err,
		)
	}

	if store.calls != 0 {
		t.Fatalf(
			"expected no lookup, got %d",
			store.calls,
		)
	}
}
