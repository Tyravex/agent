package commands

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

var (
	ErrOwnershipJournalMissing = errors.New(
		"initialized ownership revision journal is missing",
	)

	ErrOwnershipInitializationCorrupt = errors.New(
		"ownership journal initialization marker is invalid",
	)
)

const ownershipInitializationMarker = "tyravex-ownership-journal-v1\n"

// ensureOwnershipJournalInitialized prepares a new journal
// or validates and marks an existing journal.
//
// The caller must hold the OS-level ownership file lock.
//
// expectedNodeID is required because existing journals
// must be validated before migration.
func ensureOwnershipJournalInitialized(
	path string,
	expectedNodeID string,
) error {
	markerPath := path + ".initialized"

	markerData, markerErr := os.ReadFile(markerPath)

	if markerErr == nil {
		if !bytes.Equal(
			markerData,
			[]byte(ownershipInitializationMarker),
		) {
			return ErrOwnershipInitializationCorrupt
		}

		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return ErrOwnershipJournalMissing
			}

			return err
		}

		return nil
	}

	if !errors.Is(markerErr, os.ErrNotExist) {
		return markerErr
	}

	// No marker exists. Determine whether we are
	// migrating an existing journal or initializing
	// a new one.
	file, err := os.OpenFile(
		path,
		os.O_RDWR|os.O_CREATE,
		0600,
	)
	if err != nil {
		return err
	}

	// Validate the entire journal before publishing
	// the initialization marker.
	_, _, readErr := readOwnershipRevisionLog(
		file,
		expectedNodeID,
	)

	if readErr != nil {
		_ = file.Close()
		return readErr
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	// Exclusive creation prevents silently replacing
	// an existing marker.
	marker, err := os.OpenFile(
		markerPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0600,
	)
	if err != nil {
		return err
	}

	n, writeErr := marker.Write(
		[]byte(ownershipInitializationMarker),
	)

	if writeErr != nil || n != len(ownershipInitializationMarker) {
		_ = marker.Close()

		if writeErr != nil {
			return writeErr
		}

		return fmt.Errorf(
			"write ownership marker: %w",
			os.ErrInvalid,
		)
	}

	if err := marker.Sync(); err != nil {
		_ = marker.Close()
		return err
	}

	return marker.Close()
}
