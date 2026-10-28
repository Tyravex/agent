package commands

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrAuthorizationUnavailable = errors.New(
		"service authorization unavailable",
	)
	ErrServiceNotAuthorized = errors.New(
		"service is not authorized for this node",
	)
)

var authorizedServiceIDPattern = regexp.MustCompile(
	`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`,
)

// ServiceOwnership describes a trusted assignment of a
// service to an Agent node.
//
// These values must originate from a trusted local store,
// not directly from a WebSocket command payload.
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

// ServiceAuthorizer checks whether a lifecycle command
// may target a service assigned to this Agent node.
//
// It does not execute commands or modify ownership.
type ServiceAuthorizer struct {
	nodeID string
	store  ServiceOwnershipStore
}

func NewServiceAuthorizer(
	nodeID string,
	store ServiceOwnershipStore,
) *ServiceAuthorizer {
	return &ServiceAuthorizer{
		nodeID: nodeID,
		store:  store,
	}
}

func (a *ServiceAuthorizer) Authorize(
	ctx context.Context,
	command Command,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if a == nil || a.nodeID == "" || a.store == nil {
		return ErrAuthorizationUnavailable
	}

	if err := Validate(command); err != nil {
		return err
	}

	// Installation needs a separate authorization path.
	// A lifecycle assignment alone never grants permission
	// to create or install a new service.
	switch command.Operation {
	case ServiceStart, ServiceStop, ServiceRestart:
		// Supported lifecycle operations.

	default:
		return fmt.Errorf(
			"%w: operation %q",
			ErrServiceNotAuthorized,
			command.Operation,
		)
	}

	if !authorizedServiceIDPattern.MatchString(
		command.ServiceID,
	) {
		return fmt.Errorf(
			"%w: invalid service ID",
			ErrServiceNotAuthorized,
		)
	}

	ownership, err := a.store.Lookup(
		ctx,
		command.ServiceID,
	)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		// Do not disclose whether the service exists.
		return fmt.Errorf(
			"%w: ownership lookup failed",
			ErrServiceNotAuthorized,
		)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if !ownership.Enabled ||
		ownership.ServiceID != command.ServiceID ||
		ownership.NodeID != a.nodeID {
		return ErrServiceNotAuthorized
	}

	return nil
}
