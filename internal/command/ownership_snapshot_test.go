package commands

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testOwnershipSnapshot(
	revision uint64,
	serviceIDs ...string,
) OwnershipSnapshot {
	assignments := make(
		[]ServiceOwnership,
		0,
		len(serviceIDs),
	)

	for _, serviceID := range serviceIDs {
		assignments = append(
			assignments,
			ServiceOwnership{
				ServiceID: serviceID,
				NodeID:    "node-1",
				Enabled:   true,
			},
		)
	}

	return OwnershipSnapshot{
		NodeID:      "node-1",
		Revision:    revision,
		Assignments: assignments,
	}
}

func TestOwnershipSnapshotAppliesCompleteReplacement(
	t *testing.T,
) {
	ctx := context.Background()
	store := NewMemoryOwnershipStore()

	if err := store.ApplySnapshot(
		ctx,
		"node-1",
		testOwnershipSnapshot(
			1,
			"service-1",
			"service-2",
		),
	); err != nil {
		t.Fatalf("first snapshot: %v", err)
	}

	if err := store.ApplySnapshot(
		ctx,
		"node-1",
		testOwnershipSnapshot(
			2,
			"service-2",
			"service-3",
		),
	); err != nil {
		t.Fatalf("second snapshot: %v", err)
	}

	_, err := store.Lookup(ctx, "service-1")
	if !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf(
			"removed service still exists: %v",
			err,
		)
	}

	for _, serviceID := range []string{
		"service-2",
		"service-3",
	} {
		ownership, err := store.Lookup(
			ctx,
			serviceID,
		)
		if err != nil {
			t.Fatalf(
				"%s: %v",
				serviceID,
				err,
			)
		}

		if !ownership.Enabled ||
			ownership.NodeID != "node-1" {
			t.Fatalf(
				"unexpected ownership: %+v",
				ownership,
			)
		}
	}

	revision, applied := store.SnapshotRevision()
	if !applied || revision != 2 {
		t.Fatalf(
			"unexpected revision: %d, %v",
			revision,
			applied,
		)
	}
}

func TestOwnershipSnapshotRejectsStaleRevisions(
	t *testing.T,
) {
	ctx := context.Background()
	store := NewMemoryOwnershipStore()

	if err := store.ApplySnapshot(
		ctx,
		"node-1",
		testOwnershipSnapshot(10, "service-1"),
	); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}

	for _, revision := range []uint64{9, 10} {
		err := store.ApplySnapshot(
			ctx,
			"node-1",
			testOwnershipSnapshot(
				revision,
				"service-2",
			),
		)

		if !errors.Is(
			err,
			ErrStaleOwnershipRevision,
		) {
			t.Fatalf(
				"revision %d: unexpected error %v",
				revision,
				err,
			)
		}
	}

	if _, err := store.Lookup(
		ctx,
		"service-1",
	); err != nil {
		t.Fatalf(
			"original assignment lost: %v",
			err,
		)
	}

	if _, err := store.Lookup(
		ctx,
		"service-2",
	); !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf(
			"stale assignment applied: %v",
			err,
		)
	}
}

func TestOwnershipSnapshotRejectsInvalidDataAtomically(
	t *testing.T,
) {
	ctx := context.Background()
	store := NewMemoryOwnershipStore()

	if err := store.ApplySnapshot(
		ctx,
		"node-1",
		testOwnershipSnapshot(1, "service-1"),
	); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}

	tests := []struct {
		name     string
		snapshot OwnershipSnapshot
	}{
		{
			name: "wrong node",
			snapshot: OwnershipSnapshot{
				NodeID:   "node-2",
				Revision: 2,
			},
		},
		{
			name: "zero revision",
			snapshot: testOwnershipSnapshot(
				0,
				"service-2",
			),
		},
		{
			name: "duplicate service",
			snapshot: testOwnershipSnapshot(
				2,
				"service-2",
				"service-2",
			),
		},
		{
			name: "assignment for another node",
			snapshot: OwnershipSnapshot{
				NodeID:   "node-1",
				Revision: 2,
				Assignments: []ServiceOwnership{
					{
						ServiceID: "service-2",
						NodeID:    "node-2",
						Enabled:   true,
					},
				},
			},
		},
		{
			name: "invalid service ID",
			snapshot: testOwnershipSnapshot(
				2,
				"../other",
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.ApplySnapshot(
				ctx,
				"node-1",
				tt.snapshot,
			)

			if !errors.Is(
				err,
				ErrInvalidOwnershipSnapshot,
			) {
				t.Fatalf(
					"expected invalid snapshot, got %v",
					err,
				)
			}

			if _, err := store.Lookup(
				ctx,
				"service-1",
			); err != nil {
				t.Fatalf(
					"original assignment lost: %v",
					err,
				)
			}

			revision, applied := store.SnapshotRevision()
			if !applied || revision != 1 {
				t.Fatalf(
					"revision changed: %d, %v",
					revision,
					applied,
				)
			}
		})
	}
}

func TestOwnershipSnapshotEmptyListRevokesAll(
	t *testing.T,
) {
	ctx := context.Background()
	store := NewMemoryOwnershipStore()

	if err := store.ApplySnapshot(
		ctx,
		"node-1",
		testOwnershipSnapshot(
			1,
			"service-1",
			"service-2",
		),
	); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}

	if err := store.ApplySnapshot(
		ctx,
		"node-1",
		testOwnershipSnapshot(2),
	); err != nil {
		t.Fatalf("empty snapshot: %v", err)
	}

	for _, serviceID := range []string{
		"service-1",
		"service-2",
	} {
		_, err := store.Lookup(ctx, serviceID)

		if !errors.Is(err, ErrOwnershipNotFound) {
			t.Fatalf(
				"%s was not revoked: %v",
				serviceID,
				err,
			)
		}
	}
}

func TestOwnershipSnapshotWaitsForActiveLease(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	store := NewMemoryOwnershipStore()

	if err := store.ApplySnapshot(
		ctx,
		"node-1",
		testOwnershipSnapshot(1, "service-1"),
	); err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}

	leaseStarted := make(chan struct{})
	releaseLease := make(chan struct{})
	leaseDone := make(chan error, 1)

	go func() {
		leaseDone <- store.WithAuthorizedService(
			ctx,
			"node-1",
			"service-1",
			func() error {
				close(leaseStarted)
				<-releaseLease
				return nil
			},
		)
	}()

	select {
	case <-leaseStarted:
	case <-ctx.Done():
		t.Fatal("lease did not start")
	}

	snapshotDone := make(chan error, 1)

	go func() {
		snapshotDone <- store.ApplySnapshot(
			ctx,
			"node-1",
			testOwnershipSnapshot(2),
		)
	}()

	select {
	case err := <-snapshotDone:
		t.Fatalf(
			"snapshot completed during lease: %v",
			err,
		)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseLease)

	select {
	case err := <-leaseDone:
		if err != nil {
			t.Fatalf("lease: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("lease did not finish")
	}

	select {
	case err := <-snapshotDone:
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("snapshot did not finish")
	}

	_, err := store.Lookup(
		context.Background(),
		"service-1",
	)
	if !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf(
			"service not revoked: %v",
			err,
		)
	}
}

func TestOwnershipSnapshotHonorsCancellation(
	t *testing.T,
) {
	store := NewMemoryOwnershipStore()

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	err := store.ApplySnapshot(
		ctx,
		"node-1",
		testOwnershipSnapshot(1, "service-1"),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"expected cancellation, got %v",
			err,
		)
	}

	if revision, applied := store.SnapshotRevision(); applied ||
		revision != 0 {
		t.Fatalf(
			"cancelled snapshot was applied: %d, %v",
			revision,
			applied,
		)
	}
}

func TestOwnershipSnapshotNilStore(t *testing.T) {
	var store *MemoryOwnershipStore

	err := store.ApplySnapshot(
		context.Background(),
		"node-1",
		testOwnershipSnapshot(1),
	)

	if !errors.Is(
		err,
		ErrAuthorizationUnavailable,
	) {
		t.Fatalf(
			"expected unavailable store, got %v",
			err,
		)
	}
}
