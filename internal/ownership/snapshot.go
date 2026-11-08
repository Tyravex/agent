package ownership

import "errors"

var (
	ErrInvalidOwnershipSnapshot = errors.New(
		"invalid ownership snapshot",
	)
	ErrStaleOwnershipRevision = errors.New(
		"stale ownership revision",
	)
)

// OwnershipSnapshot is a complete assignment snapshot
// for one Agent node.
//
// Revision is strictly increasing within a trusted
// synchronization epoch.
//
// The transport layer must authenticate the source
// before passing a snapshot to the store.
type OwnershipSnapshot struct {
	NodeID      string
	Revision    uint64
	Assignments []ServiceOwnership
}
