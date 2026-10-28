package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnershipFileLockExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.lock")

	first, err := os.OpenFile(
		path,
		os.O_RDWR|os.O_CREATE,
		0600,
	)
	if err != nil {
		t.Fatalf("open first handle: %v", err)
	}
	defer first.Close()

	if err := lockOwnershipFile(first); err != nil {
		t.Fatalf("first lock: %v", err)
	}

	second, err := os.OpenFile(
		path,
		os.O_RDWR,
		0600,
	)
	if err != nil {
		t.Fatalf("open second handle: %v", err)
	}
	defer second.Close()

	err = lockOwnershipFile(second)
	if !errors.Is(err, ErrOwnershipStoreLocked) {
		t.Fatalf(
			"expected exclusive lock denial, got %v",
			err,
		)
	}

	if err := unlockOwnershipFile(first); err != nil {
		t.Fatalf("unlock first: %v", err)
	}

	if err := lockOwnershipFile(second); err != nil {
		t.Fatalf(
			"second lock after release: %v",
			err,
		)
	}

	if err := unlockOwnershipFile(second); err != nil {
		t.Fatalf("unlock second: %v", err)
	}
}

func TestOwnershipFileLockSurvivesExistingLockFile(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	// Simulate a lock file left behind by an
	// earlier process. No system lock is held.
	if err := os.WriteFile(
		path+".lock",
		[]byte("old lock file"),
		0600,
	); err != nil {
		t.Fatalf("create old lock file: %v", err)
	}

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf(
			"open with existing lock file: %v",
			err,
		)
	}

	if err := store.ApplySnapshot(
		context.Background(),
		testOwnershipSnapshot(1, "service-1"),
	); err != nil {
		t.Fatalf("ApplySnapshot: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Close must not remove the lock file.
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Fatalf("lock file disappeared: %v", err)
	}

	reopened, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	revision, applied := reopened.SnapshotRevision()
	if !applied || revision != 1 {
		t.Fatalf(
			"revision lost: %d, applied=%v",
			revision,
			applied,
		)
	}

	// Restart still fails closed for assignments.
	_, err = reopened.Lookup(
		context.Background(),
		"service-1",
	)
	if !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf(
			"ownership survived restart: %v",
			err,
		)
	}
}

func TestOwnershipFileLockReleasedAfterFailedOpen(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	if err := os.WriteFile(
		path,
		[]byte("invalid-json\n"),
		0600,
	); err != nil {
		t.Fatalf("write corrupt log: %v", err)
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
		t.Fatalf("unexpected error: %v", err)
	}

	// The failed open must release the system lock.
	lockFile, err := os.OpenFile(
		path+".lock",
		os.O_RDWR,
		0600,
	)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer lockFile.Close()

	if err := lockOwnershipFile(lockFile); err != nil {
		t.Fatalf(
			"failed open retained lock: %v",
			err,
		)
	}

	if err := unlockOwnershipFile(lockFile); err != nil {
		t.Fatalf("unlock: %v", err)
	}
}
