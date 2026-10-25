package transport

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistentRegistryRejectsDuplicateJSONKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.jsonl")

	data := []byte(
		`{"commandId":"first","commandId":"second"}` + "\n",
	)

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	registry, err := openPersistentCommandRegistry(path)
	if err == nil {
		_ = registry.Close()
		t.Fatal("expected corrupted journal rejection")
	}

	if !errors.Is(err, ErrCommandRegistryCorrupt) {
		t.Fatalf("expected corrupt journal, got %v", err)
	}
}

func TestPersistentRegistryRejectsOversizedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.jsonl")

	data := []byte(
		`{"commandId":"` +
			strings.Repeat("x", 128*1024) +
			`"}` + "\n",
	)

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	registry, err := openPersistentCommandRegistry(path)
	if err == nil {
		_ = registry.Close()
		t.Fatal("expected oversized journal rejection")
	}

	if !errors.Is(err, ErrCommandRegistryCorrupt) {
		t.Fatalf("expected corrupt journal, got %v", err)
	}
}

func TestPersistentRegistryRejectsNoncanonicalRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.jsonl")

	data := []byte(
		`{ "commandId": "example" }` + "\n",
	)

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	registry, err := openPersistentCommandRegistry(path)
	if err == nil {
		_ = registry.Close()
		t.Fatal("expected noncanonical journal rejection")
	}

	if !errors.Is(err, ErrCommandRegistryCorrupt) {
		t.Fatalf("expected corrupt journal, got %v", err)
	}
}

func TestPersistentRegistryAcceptsCanonicalRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.jsonl")

	data := []byte(
		`{"commandId":"example"}` + "\n",
	)

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	registry, err := openPersistentCommandRegistry(path)
	if err != nil {
		t.Fatalf("open canonical journal: %v", err)
	}
	defer registry.Close()

	if err := registry.reserve("example"); !errors.Is(
		err,
		ErrDuplicateCommandID,
	) {
		t.Fatalf("expected duplicate command ID, got %v", err)
	}

	if err := registry.reserve("new-command"); err != nil {
		t.Fatalf("reserve new command: %v", err)
	}
}
