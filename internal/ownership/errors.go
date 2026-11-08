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

// ErrOwnershipJournalMissing indicates that an initialized
// ownership journal no longer exists.
var ErrOwnershipJournalMissing = errors.New(
	"initialized ownership revision journal is missing",
)

// ErrOwnershipInitializationCorrupt indicates that the
// ownership journal initialization marker is invalid.
var ErrOwnershipInitializationCorrupt = errors.New(
	"ownership journal initialization marker is invalid",
)

// ErrOwnershipIntegrity indicates an ownership journal
// integrity verification failure.
var ErrOwnershipIntegrity = errors.New(
	"ownership revision journal integrity failure",
)

// ErrOwnershipStoreLocked indicates that another process
// already holds the exclusive ownership store lock.
var ErrOwnershipStoreLocked = errors.New(
	"persistent ownership store already locked",
)
