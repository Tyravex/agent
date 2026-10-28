package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnershipInitializationFirstStartup(t *testing.T) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf("first startup: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("journal missing: %v", err)
	}

	marker, err := os.ReadFile(path + ".initialized")
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}

	if string(marker) != ownershipInitializationMarker {
		t.Fatalf("unexpected marker: %q", marker)
	}
}

func TestOwnershipInitializationRejectsMissingJournal(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf("initial startup: %v", err)
	}

	if err := store.ApplySnapshot(
		context.Background(),
		testOwnershipSnapshot(10, "service-1"),
	); err != nil {
		t.Fatalf("ApplySnapshot: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove journal: %v", err)
	}

	reopened, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)

	if reopened != nil {
		_ = reopened.Close()
		t.Fatal("missing journal unexpectedly accepted")
	}

	if !errors.Is(err, ErrOwnershipJournalMissing) {
		t.Fatalf(
			"expected missing journal rejection, got %v",
			err,
		)
	}
}

func TestOwnershipInitializationPreservesLegacyJournal(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	data := []byte(
		"{\"node_id\":\"node-1\",\"revision\":10}\n",
	)

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf("open legacy journal: %v", err)
	}
	defer store.Close()

	revision, applied := store.SnapshotRevision()

	if !applied || revision != 10 {
		t.Fatalf(
			"legacy revision lost: %d, applied=%v",
			revision,
			applied,
		)
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(current) != string(data) {
		t.Fatal("legacy journal was modified")
	}

	marker, err := os.ReadFile(path + ".initialized")
	if err != nil {
		t.Fatalf("legacy marker missing: %v", err)
	}

	if string(marker) != ownershipInitializationMarker {
		t.Fatalf("invalid migrated marker: %q", marker)
	}
}

func TestOwnershipInitializationRejectsCorruptJournal(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	if err := os.WriteFile(
		path,
		[]byte("corrupt\n"),
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
		t.Fatal("corrupt journal unexpectedly accepted")
	}

	if !errors.Is(err, ErrOwnershipStoreCorrupt) {
		t.Fatalf(
			"expected corrupt journal rejection, got %v",
			err,
		)
	}

	// A failed migration must not publish a marker.
	if _, err := os.Stat(
		path + ".initialized",
	); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(
			"marker created for corrupt journal: %v",
			err,
		)
	}
}

func TestOwnershipInitializationRejectsInvalidMarker(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	if err := os.WriteFile(
		path,
		[]byte{},
		0600,
	); err != nil {
		t.Fatalf("write journal: %v", err)
	}

	if err := os.WriteFile(
		path+".initialized",
		[]byte("invalid-marker\n"),
		0600,
	); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)

	if store != nil {
		_ = store.Close()
		t.Fatal("invalid marker unexpectedly accepted")
	}

	if !errors.Is(
		err,
		ErrOwnershipInitializationCorrupt,
	) {
		t.Fatalf(
			"expected invalid marker rejection, got %v",
			err,
		)
	}
}

func TestOwnershipInitializationRejectsEmptyMarker(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatalf("write journal: %v", err)
	}

	if err := os.WriteFile(
		path+".initialized",
		nil,
		0600,
	); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)

	if store != nil {
		_ = store.Close()
		t.Fatal("empty marker unexpectedly accepted")
	}

	if !errors.Is(
		err,
		ErrOwnershipInitializationCorrupt,
	) {
		t.Fatalf(
			"expected corrupt marker rejection, got %v",
			err,
		)
	}
}

func TestOwnershipInitializationRecoversUnmarkedJournal(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	// Simulate an interruption after the journal was
	// created but before its marker was published.
	if err := os.WriteFile(
		path,
		nil,
		0600,
	); err != nil {
		t.Fatalf("write incomplete journal: %v", err)
	}

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)
	if err != nil {
		t.Fatalf("recover journal: %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(
		path + ".initialized",
	); err != nil {
		t.Fatalf("recovery marker missing: %v", err)
	}

	revision, applied := store.SnapshotRevision()

	if revision != 0 || applied {
		t.Fatalf(
			"unexpected revision: %d, applied=%v",
			revision,
			applied,
		)
	}
}

func TestOwnershipInitializationRejectsWrongNodeDuringMigration(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.jsonl",
	)

	data := []byte(
		"{\"node_id\":\"other-node\",\"revision\":10}\n",
	)

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := OpenPersistentOwnershipStore(
		"node-1",
		path,
	)

	if store != nil {
		_ = store.Close()
		t.Fatal("wrong-node journal unexpectedly accepted")
	}

	if !errors.Is(err, ErrOwnershipStoreCorrupt) {
		t.Fatalf(
			"expected wrong-node rejection, got %v",
			err,
		)
	}

	if _, err := os.Stat(
		path + ".initialized",
	); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(
			"marker created for wrong-node journal: %v",
			err,
		)
	}
}
