package commands

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestMemoryOwnershipStoreStartsEmpty(t *testing.T) {
	store := NewMemoryOwnershipStore()

	_, err := store.Lookup(
		context.Background(),
		"service-1",
	)

	if !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf(
			"expected missing ownership, got %v",
			err,
		)
	}
}

func TestMemoryOwnershipStoreUpsertAndLookup(t *testing.T) {
	store := NewMemoryOwnershipStore()
	ctx := context.Background()

	expected := ServiceOwnership{
		ServiceID: "service-1",
		NodeID:    "node-1",
		Enabled:   true,
	}

	if err := store.Upsert(ctx, expected); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	actual, err := store.Lookup(
		ctx,
		"service-1",
	)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	if actual != expected {
		t.Fatalf(
			"expected %+v, got %+v",
			expected,
			actual,
		)
	}
}

func TestMemoryOwnershipStoreUpdatesAssignment(t *testing.T) {
	store := NewMemoryOwnershipStore()
	ctx := context.Background()

	if err := store.Upsert(ctx, ServiceOwnership{
		ServiceID: "service-1",
		NodeID:    "node-1",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("initial Upsert: %v", err)
	}

	if err := store.Upsert(ctx, ServiceOwnership{
		ServiceID: "service-1",
		NodeID:    "node-2",
		Enabled:   false,
	}); err != nil {
		t.Fatalf("updated Upsert: %v", err)
	}

	actual, err := store.Lookup(ctx, "service-1")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	if actual.NodeID != "node-2" || actual.Enabled {
		t.Fatalf(
			"assignment was not replaced: %+v",
			actual,
		)
	}
}

func TestMemoryOwnershipStoreRevoke(t *testing.T) {
	store := NewMemoryOwnershipStore()
	ctx := context.Background()

	if err := store.Upsert(ctx, ServiceOwnership{
		ServiceID: "service-1",
		NodeID:    "node-1",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := store.Revoke(ctx, "service-1"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Revocation is idempotent.
	if err := store.Revoke(ctx, "service-1"); err != nil {
		t.Fatalf("second Revoke: %v", err)
	}

	_, err := store.Lookup(ctx, "service-1")
	if !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf(
			"expected revoked ownership, got %v",
			err,
		)
	}
}

func TestMemoryOwnershipStoreRejectsInvalidAssignments(
	t *testing.T,
) {
	tests := []struct {
		name      string
		ownership ServiceOwnership
	}{
		{
			name: "missing service ID",
			ownership: ServiceOwnership{
				NodeID:  "node-1",
				Enabled: true,
			},
		},
		{
			name: "unsafe service ID",
			ownership: ServiceOwnership{
				ServiceID: "../other",
				NodeID:    "node-1",
				Enabled:   true,
			},
		},
		{
			name: "missing node ID",
			ownership: ServiceOwnership{
				ServiceID: "service-1",
				Enabled:   true,
			},
		},
		{
			name: "node ID with whitespace",
			ownership: ServiceOwnership{
				ServiceID: "service-1",
				NodeID:    " node-1 ",
				Enabled:   true,
			},
		},
		{
			name: "node ID with newline",
			ownership: ServiceOwnership{
				ServiceID: "service-1",
				NodeID:    "node-1\nother",
				Enabled:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryOwnershipStore()

			err := store.Upsert(
				context.Background(),
				tt.ownership,
			)

			if !errors.Is(
				err,
				ErrInvalidOwnership,
			) {
				t.Fatalf(
					"expected invalid ownership, got %v",
					err,
				)
			}
		})
	}
}

func TestMemoryOwnershipStoreRejectsInvalidLookupAndRevoke(
	t *testing.T,
) {
	store := NewMemoryOwnershipStore()
	ctx := context.Background()

	_, err := store.Lookup(ctx, "../other")
	if !errors.Is(err, ErrInvalidOwnership) {
		t.Fatalf(
			"expected invalid lookup, got %v",
			err,
		)
	}

	err = store.Revoke(ctx, "../other")
	if !errors.Is(err, ErrInvalidOwnership) {
		t.Fatalf(
			"expected invalid revoke, got %v",
			err,
		)
	}
}

func TestMemoryOwnershipStoreHonorsCancellation(
	t *testing.T,
) {
	store := NewMemoryOwnershipStore()

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	ownership := ServiceOwnership{
		ServiceID: "service-1",
		NodeID:    "node-1",
		Enabled:   true,
	}

	if err := store.Upsert(ctx, ownership); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("Upsert: expected cancellation, got %v", err)
	}

	if _, err := store.Lookup(
		ctx,
		"service-1",
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("Lookup: expected cancellation, got %v", err)
	}

	if err := store.Revoke(
		ctx,
		"service-1",
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("Revoke: expected cancellation, got %v", err)
	}

	_, err := store.Lookup(
		context.Background(),
		"service-1",
	)
	if !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf(
			"cancelled Upsert modified registry: %v",
			err,
		)
	}
}

func TestMemoryOwnershipStoreNilReceiver(t *testing.T) {
	var store *MemoryOwnershipStore
	ctx := context.Background()

	if _, err := store.Lookup(
		ctx,
		"service-1",
	); !errors.Is(err, ErrAuthorizationUnavailable) {
		t.Fatalf("Lookup: unexpected error %v", err)
	}

	if err := store.Upsert(
		ctx,
		ServiceOwnership{
			ServiceID: "service-1",
			NodeID:    "node-1",
			Enabled:   true,
		},
	); !errors.Is(err, ErrAuthorizationUnavailable) {
		t.Fatalf("Upsert: unexpected error %v", err)
	}

	if err := store.Revoke(
		ctx,
		"service-1",
	); !errors.Is(err, ErrAuthorizationUnavailable) {
		t.Fatalf("Revoke: unexpected error %v", err)
	}
}

func TestMemoryOwnershipStoreConcurrentAccess(t *testing.T) {
	store := NewMemoryOwnershipStore()
	ctx := context.Background()

	const workers = 16
	const iterations = 100

	var wg sync.WaitGroup

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)

		go func(worker int) {
			defer wg.Done()

			serviceID := fmt.Sprintf(
				"service-%d",
				worker,
			)

			for i := 0; i < iterations; i++ {
				if err := store.Upsert(
					ctx,
					ServiceOwnership{
						ServiceID: serviceID,
						NodeID:    "node-1",
						Enabled:   true,
					},
				); err != nil {
					t.Errorf("Upsert: %v", err)
					return
				}

				if _, err := store.Lookup(
					ctx,
					serviceID,
				); err != nil {
					t.Errorf("Lookup: %v", err)
					return
				}

				if err := store.Revoke(
					ctx,
					serviceID,
				); err != nil {
					t.Errorf("Revoke: %v", err)
					return
				}
			}
		}(worker)
	}

	wg.Wait()

	for worker := 0; worker < workers; worker++ {
		serviceID := fmt.Sprintf(
			"service-%d",
			worker,
		)

		_, err := store.Lookup(ctx, serviceID)
		if !errors.Is(err, ErrOwnershipNotFound) {
			t.Fatalf(
				"%s: expected revoked assignment, got %v",
				serviceID,
				err,
			)
		}
	}
}

func TestMemoryOwnershipStoreIntegratesWithAuthorizer(
	t *testing.T,
) {
	ctx := context.Background()
	store := NewMemoryOwnershipStore()

	authorizer := NewServiceAuthorizer(
		"node-1",
		store,
	)

	command := Command{
		ID:        "command-1",
		ServiceID: "service-1",
		Operation: ServiceStart,
	}

	// No assignment: deny.
	if err := authorizer.Authorize(
		ctx,
		command,
	); !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf("expected initial denial, got %v", err)
	}

	// Trusted assignment: allow.
	if err := store.Upsert(
		ctx,
		ServiceOwnership{
			ServiceID: "service-1",
			NodeID:    "node-1",
			Enabled:   true,
		},
	); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := authorizer.Authorize(
		ctx,
		command,
	); err != nil {
		t.Fatalf("expected authorization, got %v", err)
	}

	// Reassignment to another node: deny.
	if err := store.Upsert(
		ctx,
		ServiceOwnership{
			ServiceID: "service-1",
			NodeID:    "node-2",
			Enabled:   true,
		},
	); err != nil {
		t.Fatalf("reassignment: %v", err)
	}

	if err := authorizer.Authorize(
		ctx,
		command,
	); !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf("expected denial after reassignment, got %v", err)
	}

	// Revocation: deny.
	if err := store.Revoke(
		ctx,
		"service-1",
	); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if err := authorizer.Authorize(
		ctx,
		command,
	); !errors.Is(err, ErrServiceNotAuthorized) {
		t.Fatalf("expected denial after revoke, got %v", err)
	}
}
