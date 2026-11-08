package ownership

import "errors"

// ErrAuthorizationUnavailable indicates that the
// ownership authorization dependency is unavailable.
var ErrAuthorizationUnavailable = errors.New(
	"service authorization unavailable",
)

// ErrServiceNotAuthorized indicates that the requested
// service operation is not authorized for this node.
var ErrServiceNotAuthorized = errors.New(
	"service is not authorized for this node",
)

// ErrLeaseUnavailable indicates that the ownership
// lease dependency is unavailable.
var ErrLeaseUnavailable = errors.New(
	"service lease unavailable",
)

// ErrOwnershipNotFound indicates that no assignment
// exists for the requested service.
var ErrOwnershipNotFound = errors.New(
	"service ownership not found",
)

// ErrOwnershipStoreCorrupt indicates an invalid revision journal.
var ErrOwnershipStoreCorrupt = errors.New(
	"persistent ownership store is corrupt",
)
