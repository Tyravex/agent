package commands

import "github.com/Tyravex/agent/internal/ownership"

// Compatibility aliases preserve existing ownership errors.
var (
	ErrOwnershipNotFound = ownership.ErrOwnershipNotFound
	ErrInvalidOwnership  = ownership.ErrInvalidOwnership
)

// MemoryOwnershipStore remains available to existing callers.
type MemoryOwnershipStore = ownership.MemoryOwnershipStore

// NewMemoryOwnershipStore preserves the existing constructor.
func NewMemoryOwnershipStore() *MemoryOwnershipStore {
	return ownership.NewMemoryOwnershipStore()
}

func validateOwnership(assignment ServiceOwnership) error {
	return ownership.ValidateAssignment(assignment)
}
