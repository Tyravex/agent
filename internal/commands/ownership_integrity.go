package commands

import (
	"os"

	"github.com/Tyravex/agent/internal/ownership"
)

// Compatibility aliases preserve existing error identity
// and package-local references.
var ErrOwnershipIntegrity = ownership.ErrOwnershipIntegrity

type ownershipIntegrityRecord = ownership.IntegrityRecord

func readOwnershipJournalHash(
	file *os.File,
) (string, error) {
	return ownership.ReadJournalHash(file)
}

func readOwnershipIntegrity(
	path string,
) (ownershipIntegrityRecord, error) {
	return ownership.ReadIntegrity(path)
}

func writeOwnershipIntegrity(
	path string,
	record ownershipIntegrityRecord,
) error {
	return ownership.WriteIntegrity(path, record)
}
