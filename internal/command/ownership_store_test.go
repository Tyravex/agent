package commands

import (
	"context"
	"errors"
	"testing"
)

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
