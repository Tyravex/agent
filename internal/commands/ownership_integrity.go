package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

var ErrOwnershipIntegrity = errors.New(
	"ownership revision journal integrity failure",
)

type ownershipIntegrityRecord struct {
	NodeID   string `json:"node_id"`
	Revision uint64 `json:"revision"`
	SHA256   string `json:"sha256"`
}

// readOwnershipJournalHash calculates the hash of the
// complete journal without changing the caller's file
// position.
func readOwnershipJournalHash(
	file *os.File,
) (string, error) {
	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", err
	}

	defer func() {
		_, _ = file.Seek(position, io.SeekStart)
	}()

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	hash := sha256.New()

	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readOwnershipIntegrity(
	path string,
) (ownershipIntegrityRecord, error) {
	var record ownershipIntegrityRecord

	data, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}

	if err := json.Unmarshal(data, &record); err != nil {
		return record, fmt.Errorf(
			"%w: invalid integrity metadata",
			ErrOwnershipIntegrity,
		)
	}

	if record.NodeID == "" ||
		record.Revision == 0 ||
		len(record.SHA256) != sha256.Size*2 {
		return record, ErrOwnershipIntegrity
	}

	decoded, err := hex.DecodeString(record.SHA256)
	if err != nil || len(decoded) != sha256.Size {
		return record, ErrOwnershipIntegrity
	}

	return record, nil
}

func writeOwnershipIntegrity(
	path string,
	record ownershipIntegrityRecord,
) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	data = append(data, '\n')

	// Use a separate temporary file so that a failed
	// write does not partially overwrite valid metadata.
	tempPath := path + ".tmp"

	file, err := os.OpenFile(
		tempPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0600,
	)
	if err != nil {
		return err
	}

	success := false

	defer func() {
		_ = file.Close()

		if !success {
			_ = os.Remove(tempPath)
		}
	}()

	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}

	if err := file.Sync(); err != nil {
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	if err := os.Rename(tempPath, path); err != nil {
		return err
	}

	success = true
	return nil
}
