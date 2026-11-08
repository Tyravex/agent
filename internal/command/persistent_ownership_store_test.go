package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openTestPersistentStore(
	t *testing.T,
	path string,
) *PersistentOwnershipStore {
	t.Helper()

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf("OpenPersistentOwnershipStore: %v", err)
	}

	return store
}

func TestPersistentOwnershipStoreRestartFailsClosed(
	t *testing.T,
) {
	ctx := context.Background()
	path := filepath.Join(
		t.TempDir(),
		"ownership-revisions.jsonl",
	)

	store := openTestPersistentStore(t, path)

	if err := store.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(10, "service-1"),
	); err != nil {
		t.Fatalf("ApplySnapshot: %v", err)
	}

	if _, err := store.Lookup(
		ctx,
		"service-1",
	); err != nil {
		t.Fatalf("Lookup before restart: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened := openTestPersistentStore(t, path)
	defer reopened.Close()

	revision, applied := reopened.SnapshotRevision()

	if !applied || revision != 10 {
		t.Fatalf(
			"unexpected restored revision: %d, %v",
			revision,
			applied,
		)
	}

	// Ownership must not survive the restart.
	if _, err := reopened.Lookup(
		ctx,
		"service-1",
	); !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf(
			"service authorized after restart: %v",
			err,
		)
	}

	// An older snapshot cannot restore old access.
	err := reopened.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(9, "service-1"),
	)

	if !errors.Is(err, ErrStaleOwnershipRevision) {
		t.Fatalf(
			"expected stale revision, got %v",
			err,
		)
	}

	// Even an identical revision is rejected.
	err = reopened.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(10, "service-1"),
	)

	if !errors.Is(err, ErrStaleOwnershipRevision) {
		t.Fatalf(
			"expected duplicate revision denial, got %v",
			err,
		)
	}

	// A newer trusted snapshot restores access.
	if err := reopened.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(11, "service-1"),
	); err != nil {
		t.Fatalf("new snapshot: %v", err)
	}

	if _, err := reopened.Lookup(
		ctx,
		"service-1",
	); err != nil {
		t.Fatalf(
			"service not restored: %v",
			err,
		)
	}
}

func TestPersistentOwnershipStoreRejectsConcurrentOpen(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"ownership-revisions.jsonl",
	)

	first := openTestPersistentStore(t, path)
	defer first.Close()

	second, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)

	if second != nil {
		_ = second.Close()
		t.Fatal("second store unexpectedly opened")
	}

	if !errors.Is(err, ErrOwnershipStoreLocked) {
		t.Fatalf(
			"expected locked store, got %v",
			err,
		)
	}
}

func TestPersistentOwnershipStoreRejectsWrongNode(
	t *testing.T,
) {
	ctx := context.Background()
	path := filepath.Join(
		t.TempDir(),
		"ownership-revisions.jsonl",
	)

	store := openTestPersistentStore(t, path)

	if err := store.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(1, "service-1"),
	); err != nil {
		t.Fatalf("ApplySnapshot: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	other, err := OpenPersistentOwnershipStore(
		"node-2",
		path,
	)

	if other != nil {
		_ = other.Close()
		t.Fatal("other node opened revision log")
	}

	if !errors.Is(err, ErrOwnershipStoreCorrupt) {
		t.Fatalf(
			"expected node mismatch rejection, got %v",
			err,
		)
	}
}

func TestPersistentOwnershipStoreRejectsCorruptLog(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"ownership-revisions.jsonl",
	)

	if err := os.WriteFile(
		path,
		[]byte(
			"{\"node_id\":\"node-1\",\"revision\":1}\n"+
				"{\"node_id\":\"node-1\",\"revision\":",
		),
		0600,
	); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)

	if store != nil {
		_ = store.Close()
		t.Fatal("corrupt store unexpectedly opened")
	}

	if !errors.Is(err, ErrOwnershipStoreCorrupt) {
		t.Fatalf(
			"expected corrupt log rejection, got %v",
			err,
		)
	}
}

func TestPersistentOwnershipStoreRejectsRevisionRollback(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"ownership-revisions.jsonl",
	)

	data := []byte(
		"{\"node_id\":\"node-1\",\"revision\":10}\n" +
			"{\"node_id\":\"node-1\",\"revision\":9}\n",
	)

	if err := os.WriteFile(
		path,
		data,
		0600,
	); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)

	if store != nil {
		_ = store.Close()
		t.Fatal("rollback log unexpectedly opened")
	}

	if !errors.Is(err, ErrOwnershipStoreCorrupt) {
		t.Fatalf(
			"expected corrupt log, got %v",
			err,
		)
	}
}

func TestPersistentOwnershipStoreClosedFailsClosed(
	t *testing.T,
) {
	ctx := context.Background()
	path := filepath.Join(
		t.TempDir(),
		"ownership-revisions.jsonl",
	)

	store := openTestPersistentStore(t, path)

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err := store.ApplySnapshot(
		ctx,
		testOwnershipSnapshot(1, "service-1"),
	)

	if !errors.Is(err, ErrOwnershipStoreClosed) {
		t.Fatalf(
			"expected closed store, got %v",
			err,
		)
	}

	_, err = store.Lookup(ctx, "service-1")

	if !errors.Is(err, ErrOwnershipStoreClosed) {
		t.Fatalf(
			"expected closed lookup, got %v",
			err,
		)
	}
}

func TestPersistentOwnershipStoreRejectsInvalidSnapshot(
	t *testing.T,
) {
	ctx := context.Background()
	path := filepath.Join(
		t.TempDir(),
		"ownership-revisions.jsonl",
	)

	store := openTestPersistentStore(t, path)
	defer store.Close()

	invalid := testOwnershipSnapshot(
		1,
		"service-1",
		"service-1",
	)

	err := store.ApplySnapshot(ctx, invalid)

	if !errors.Is(
		err,
		ErrInvalidOwnershipSnapshot,
	) {
		t.Fatalf(
			"expected invalid snapshot, got %v",
			err,
		)
	}

	revision, applied := store.SnapshotRevision()

	if applied || revision != 0 {
		t.Fatalf(
			"invalid snapshot changed revision: %d, %v",
			revision,
			applied,
		)
	}
}
