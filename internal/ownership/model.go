package ownership

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
