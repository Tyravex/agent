package commands

import "github.com/Tyravex/agent/internal/ownership"

// These aliases preserve the existing error identities.
var (
	ErrInvalidOwnershipSnapshot = ownership.ErrInvalidOwnershipSnapshot
	ErrStaleOwnershipRevision   = ownership.ErrStaleOwnershipRevision
)

// OwnershipSnapshot remains available to existing callers.
type OwnershipSnapshot = ownership.OwnershipSnapshot
