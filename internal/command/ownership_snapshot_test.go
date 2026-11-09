package commands

func testOwnershipSnapshot(
	revision uint64,
	serviceIDs ...string,
) OwnershipSnapshot {
	assignments := make(
		[]ServiceOwnership,
		0,
		len(serviceIDs),
	)

	for _, serviceID := range serviceIDs {
		assignments = append(
			assignments,
			ServiceOwnership{
				ServiceID: serviceID,
				NodeID:    "node-1",
				Enabled:   true,
			},
		)
	}

	return OwnershipSnapshot{
		NodeID:      "node-1",
		Revision:    revision,
		Assignments: assignments,
	}
}
