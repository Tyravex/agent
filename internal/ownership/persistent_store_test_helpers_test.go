package ownership

import "testing"

func openTestPersistentStore(
	t *testing.T,
	path string,
) *PersistentOwnershipStore {
	t.Helper()

	store, err := OpenPersistentOwnershipStore("node-1", path)
	if err != nil {
		t.Fatalf("OpenPersistentOwnershipStore: %v", err)
	}

	return store
}

func testOwnershipSnapshot(
	revision uint64,
	serviceIDs ...string,
) OwnershipSnapshot {
	assignments := make([]ServiceOwnership, 0, len(serviceIDs))

	for _, serviceID := range serviceIDs {
		assignments = append(assignments, ServiceOwnership{
			ServiceID: serviceID,
			NodeID:    "node-1",
			Enabled:   true,
		})
	}

	return OwnershipSnapshot{
		NodeID:      "node-1",
		Revision:    revision,
		Assignments: assignments,
	}
}
