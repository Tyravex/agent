package transport

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentCommandRegistrySurvivesReopen(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	first, err := openPersistentCommandRegistry(path)
	if err != nil {
		t.Fatalf("open first registry: %v", err)
	}

	if err := first.reserve("command-1"); err != nil {
		t.Fatalf("reserve command: %v", err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first registry: %v", err)
	}

	second, err := openPersistentCommandRegistry(path)
	if err != nil {
		t.Fatalf("reopen registry: %v", err)
	}
	defer second.Close()

	err = second.reserve("command-1")
	if !errors.Is(err, ErrDuplicateCommandID) {
		t.Fatalf(
			"expected duplicate after reopen, got %v",
			err,
		)
	}

	if err := second.reserve("command-2"); err != nil {
		t.Fatalf(
			"reserve new command after reopen: %v",
			err,
		)
	}
}

func TestPersistentCommandRegistryRejectsConcurrentOpen(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	first, err := openPersistentCommandRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = openPersistentCommandRegistry(path)
	if !errors.Is(err, ErrCommandRegistryLocked) {
		first.Close()

		t.Fatalf(
			"expected locked registry, got %v",
			err,
		)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := openPersistentCommandRegistry(path)
	if err != nil {
		t.Fatalf(
			"reopen after lock release: %v",
			err,
		)
	}
	defer second.Close()
}

func TestPersistentCommandRegistryRejectsCorruption(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	if err := os.WriteFile(
		path,
		[]byte("{invalid json}\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	_, err := openPersistentCommandRegistry(path)
	if !errors.Is(err, ErrCommandRegistryCorrupt) {
		t.Fatalf(
			"expected corruption error, got %v",
			err,
		)
	}
}

func TestPersistentCommandRegistryRejectsIncompleteRecord(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	if err := os.WriteFile(
		path,
		[]byte(
			"{\"commandId\":\"command-1\"}\n"+
				"{\"commandId\":\"command-2\"",
		),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	_, err := openPersistentCommandRegistry(path)
	if !errors.Is(err, ErrCommandRegistryCorrupt) {
		t.Fatalf(
			"expected incomplete record error, got %v",
			err,
		)
	}
}

func TestPersistentCommandRegistryRejectsDuplicateRecords(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	data := []byte(
		"{\"commandId\":\"same\"}\n" +
			"{\"commandId\":\"same\"}\n",
	)

	if err := os.WriteFile(
		path,
		data,
		0600,
	); err != nil {
		t.Fatal(err)
	}

	_, err := openPersistentCommandRegistry(path)
	if !errors.Is(err, ErrCommandRegistryCorrupt) {
		t.Fatalf(
			"expected duplicate record corruption, got %v",
			err,
		)
	}
}

func TestPersistentCommandRegistryRejectsMissingDirectory(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"missing",
		"commands.jsonl",
	)

	_, err := openPersistentCommandRegistry(path)
	if err == nil {
		t.Fatal(
			"expected error for missing directory",
		)
	}
}

func TestPersistentCommandRegistryFailsClosedAfterClose(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	registry, err := openPersistentCommandRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}

	if err := registry.reserve("command-1"); !errors.Is(
		err,
		ErrCommandRegistryClosed,
	) {
		t.Fatalf(
			"expected closed registry, got %v",
			err,
		)
	}
}

func TestPersistentCommandRegistryRetainsExistingLock(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	lockPath := path + ".lock"

	if err := os.WriteFile(
		lockPath,
		[]byte("existing lock"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	_, err := openPersistentCommandRegistry(path)
	if !errors.Is(err, ErrCommandRegistryLocked) {
		t.Fatalf(
			"expected lock error, got %v",
			err,
		)
	}

	// A failed open must never remove another
	// process's lock.
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "existing lock" {
		t.Fatal(
			"existing lock was modified",
		)
	}
}

func TestPersistentCommandRegistryReleasesLockAfterInvalidJournal(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	if err := os.WriteFile(
		path,
		[]byte("invalid\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	_, err := openPersistentCommandRegistry(path)
	if !errors.Is(err, ErrCommandRegistryCorrupt) {
		t.Fatalf(
			"expected corruption, got %v",
			err,
		)
	}

	_, err = os.Stat(path + ".lock")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(
			"lock remained after failed open: %v",
			err,
		)
	}
}
