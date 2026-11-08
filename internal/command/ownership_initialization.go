package commands

import "github.com/Tyravex/agent/internal/ownership"

// Keep the journal marker available to existing package tests.
const ownershipInitializationMarker = ownership.InitializationMarker

// Compatibility aliases preserve error identity.
var (
	ErrOwnershipJournalMissing        = ownership.ErrOwnershipJournalMissing
	ErrOwnershipInitializationCorrupt = ownership.ErrOwnershipInitializationCorrupt
)

func ensureOwnershipJournalInitialized(
	path string,
	expectedNodeID string,
) error {
	return ownership.EnsureJournalInitialized(
		path,
		expectedNodeID,
	)
}
