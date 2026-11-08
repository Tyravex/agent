package ownership

import "fmt"

// ValidateSnapshot checks a complete ownership snapshot
// and returns its assignments indexed by service ID.
//
// The returned map is built before any store mutation.
// Invalid snapshots must never partially update ownership.
func ValidateSnapshot(
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

	for _, assignment := range snapshot.Assignments {
		if err := ValidateAssignment(assignment); err != nil {
			return nil, fmt.Errorf(
				"%w: %v",
				ErrInvalidOwnershipSnapshot,
				err,
			)
		}

		if assignment.NodeID != snapshot.NodeID {
			return nil, fmt.Errorf(
				"%w: assignment targets another node",
				ErrInvalidOwnershipSnapshot,
			)
		}

		if _, exists := assignments[assignment.ServiceID]; exists {
			return nil, fmt.Errorf(
				"%w: duplicate service ID",
				ErrInvalidOwnershipSnapshot,
			)
		}

		assignments[assignment.ServiceID] = assignment
	}

	return assignments, nil
}
