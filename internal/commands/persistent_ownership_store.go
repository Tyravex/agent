package commands

import "github.com/Tyravex/agent/internal/ownership"

// Compatibility aliases preserve existing error identities.
var (
	ErrOwnershipStoreLocked   = ownership.ErrOwnershipStoreLocked
	ErrOwnershipStoreCorrupt  = ownership.ErrOwnershipStoreCorrupt
	ErrOwnershipStoreClosed   = ownership.ErrOwnershipStoreClosed
	ErrOwnershipStorePoisoned = ownership.ErrOwnershipStorePoisoned
)

// PersistentOwnershipStore remains available to existing callers.
type PersistentOwnershipStore = ownership.PersistentOwnershipStore

// OpenPersistentOwnershipStore preserves the existing constructor.
func OpenPersistentOwnershipStore(
	nodeID string,
	path string,
) (*PersistentOwnershipStore, error) {
	return ownership.OpenPersistentOwnershipStore(nodeID, path)
}
