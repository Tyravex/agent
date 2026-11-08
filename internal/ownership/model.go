package ownership

import "context"

// ServiceOwnership describes a trusted assignment
// of a service to an Agent node.
//
// Assignments must originate from a trusted store,
// never directly from an untrusted command payload.
type ServiceOwnership struct {
	ServiceID string
	NodeID    string
	Enabled   bool
}

// ServiceOwnershipStore reads trusted service assignments.
//
// A missing service must return an error. Implementations
// must not create an assignment during Lookup.
type ServiceOwnershipStore interface {
	Lookup(
		ctx context.Context,
		serviceID string,
	) (ServiceOwnership, error)
}
