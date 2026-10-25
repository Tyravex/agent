package transport

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tyravex/agent/internal/config"
)

func TestPersistentClientRegistrySurvivesRestart(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	first, err := NewClientWithPersistentRegistry(
		config.Config{},
		path,
	)
	if err != nil {
		t.Fatalf("create first client: %v", err)
	}

	if err := first.commandRegistry.reserve(
		"restart-command-1",
	); err != nil {
		t.Fatalf("reserve command: %v", err)
	}

	if err := first.ClosePersistentRegistry(); err != nil {
		t.Fatalf("close first registry: %v", err)
	}

	second, err := NewClientWithPersistentRegistry(
		config.Config{},
		path,
	)
	if err != nil {
		t.Fatalf("create second client: %v", err)
	}
	defer second.ClosePersistentRegistry()

	err = second.commandRegistry.reserve(
		"restart-command-1",
	)
	if !errors.Is(err, ErrDuplicateCommandID) {
		t.Fatalf(
			"expected duplicate after restart, got %v",
			err,
		)
	}

	if err := second.commandRegistry.reserve(
		"restart-command-2",
	); err != nil {
		t.Fatalf(
			"reserve new command: %v",
			err,
		)
	}
}

func TestPersistentClientRegistryRejectsConcurrentOwner(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	first, err := NewClientWithPersistentRegistry(
		config.Config{},
		path,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer first.ClosePersistentRegistry()

	_, err = NewClientWithPersistentRegistry(
		config.Config{},
		path,
	)
	if !errors.Is(err, ErrCommandRegistryLocked) {
		t.Fatalf(
			"expected registry lock error, got %v",
			err,
		)
	}
}

func TestPersistentClientRegistryRejectsCorruptJournal(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	if err := os.WriteFile(
		path,
		[]byte("invalid journal\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	_, err := NewClientWithPersistentRegistry(
		config.Config{},
		path,
	)
	if !errors.Is(err, ErrCommandRegistryCorrupt) {
		t.Fatalf(
			"expected corruption error, got %v",
			err,
		)
	}
}

func TestPersistentClientRegistryFailsClosedAfterClose(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"commands.jsonl",
	)

	client, err := NewClientWithPersistentRegistry(
		config.Config{},
		path,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := client.ClosePersistentRegistry(); err != nil {
		t.Fatal(err)
	}

	err = client.commandRegistry.reserve(
		"closed-command",
	)
	if !errors.Is(err, ErrCommandRegistryClosed) {
		t.Fatalf(
			"expected closed registry error, got %v",
			err,
		)
	}
}

func TestDefaultClientUsesMemoryRegistry(
	t *testing.T,
) {
	client := NewClient(config.Config{})

	if _, ok := client.commandRegistry.(*commandIDRegistry); !ok {
		t.Fatal(
			"default client must use in-memory registry",
		)
	}

	if err := client.ClosePersistentRegistry(); err != nil {
		t.Fatalf(
			"close memory-backed client: %v",
			err,
		)
	}
}
