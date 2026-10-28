package commands

import (
	"errors"
	"fmt"
)

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

func validateOwnershipSnapshot(
	snapshot OwnershipSnapshot,
	expectedNodeID string,
) (map[string]ServiceOwnership, error) {
	if snapshot.NodeID == "" ||
		snapshot.NodeID != expectedNodeID {
		return nil, fmt.Errorf(
			"%w: unexpected node ID",
			ErrInvalidOwnershipSnapshot,
		)
	}

	if snapshot.Revision == 0 {
		return nil, fmt.Errorf(
			"%w: revision must be positive",
			ErrInvalidOwnershipSnapshot,
		)
	}

	assignments := make(
		map[string]ServiceOwnership,
		len(snapshot.Assignments),
	)

	for _, ownership := range snapshot.Assignments {
		if err := validateOwnership(ownership); err != nil {
			return nil, fmt.Errorf(
				"%w: %v",
				ErrInvalidOwnershipSnapshot,
				err,
			)
		}

		if ownership.NodeID != snapshot.NodeID {
			return nil, fmt.Errorf(
				"%w: assignment targets another node",
				ErrInvalidOwnershipSnapshot,
			)
		}

		if _, exists := assignments[ownership.ServiceID]; exists {
			return nil, fmt.Errorf(
				"%w: duplicate service ID",
				ErrInvalidOwnershipSnapshot,
			)
		}

		assignments[ownership.ServiceID] = ownership
	}

	return assignments, nil
}
