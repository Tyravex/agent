package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnershipIntegrityJournalHash(t *testing.T) {
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

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()

	hash, err := readOwnershipJournalHash(file)
	if err != nil {
		t.Fatalf("readOwnershipJournalHash: %v", err)
	}

	expected := sha256.Sum256(data)

	if hash != hex.EncodeToString(expected[:]) {
		t.Fatalf("unexpected journal hash: %s", hash)
	}
}

func TestOwnershipIntegrityDetectsModifiedJournal(
	t *testing.T,
) {
	dir := t.TempDir()

	journalPath := filepath.Join(
		dir,
		"revisions.jsonl",
	)

	integrityPath := journalPath + ".integrity"

	original := []byte(
		"{\"node_id\":\"node-1\",\"revision\":10}\n",
	)

	if err := os.WriteFile(
		journalPath,
		original,
		0600,
	); err != nil {
		t.Fatalf("write journal: %v", err)
	}

	file, err := os.Open(journalPath)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}

	hash, err := readOwnershipJournalHash(file)
	_ = file.Close()

	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if err := writeOwnershipIntegrity(
		integrityPath,
		ownershipIntegrityRecord{
			NodeID:   "node-1",
			Revision: 10,
			SHA256:   hash,
		},
	); err != nil {
		t.Fatalf("write integrity: %v", err)
	}

	// Replace the journal with valid JSON containing
	// an older revision.
	modified := []byte(
		"{\"node_id\":\"node-1\",\"revision\":9}\n",
	)

	if err := os.WriteFile(
		journalPath,
		modified,
		0600,
	); err != nil {
		t.Fatalf("modify journal: %v", err)
	}

	metadata, err := readOwnershipIntegrity(
		integrityPath,
	)
	if err != nil {
		t.Fatalf("read integrity: %v", err)
	}

	file, err = os.Open(journalPath)
	if err != nil {
		t.Fatalf("reopen journal: %v", err)
	}
	defer file.Close()

	currentHash, err := readOwnershipJournalHash(file)
	if err != nil {
		t.Fatalf("current hash: %v", err)
	}

	if currentHash == metadata.SHA256 {
		t.Fatal("modified journal was not detected")
	}
}

func TestOwnershipIntegrityRejectsInvalidMetadata(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.integrity",
	)

	if err := os.WriteFile(
		path,
		[]byte(`{"node_id":"node-1","revision":10,"sha256":"bad"}`),
		0600,
	); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := readOwnershipIntegrity(path)

	if !errors.Is(err, ErrOwnershipIntegrity) {
		t.Fatalf(
			"expected integrity failure, got %v",
			err,
		)
	}
}

func TestOwnershipIntegrityMissingMetadata(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"missing.integrity",
	)

	_, err := readOwnershipIntegrity(path)

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(
			"expected missing file, got %v",
			err,
		)
	}
}

func TestOwnershipIntegrityWriteAndRead(t *testing.T) {
	path := filepath.Join(
		t.TempDir(),
		"revisions.integrity",
	)

	record := ownershipIntegrityRecord{
		NodeID:   "node-1",
		Revision: 42,
		SHA256:   hex.EncodeToString(make([]byte, sha256.Size)),
	}

	if err := writeOwnershipIntegrity(
		path,
		record,
	); err != nil {
		t.Fatalf("writeOwnershipIntegrity: %v", err)
	}

	loaded, err := readOwnershipIntegrity(path)
	if err != nil {
		t.Fatalf("readOwnershipIntegrity: %v", err)
	}

	if loaded != record {
		t.Fatalf(
			"metadata mismatch: got %+v, want %+v",
			loaded,
			record,
		)
	}
}
