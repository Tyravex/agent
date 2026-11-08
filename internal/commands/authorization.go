package commands

import (
	"context"
	"fmt"

	"github.com/Tyravex/agent/internal/ownership"
)

// Compatibility aliases preserve error identity for existing callers.
var (
	ErrAuthorizationUnavailable = ownership.ErrAuthorizationUnavailable
	ErrServiceNotAuthorized     = ownership.ErrServiceNotAuthorized
)

// ServiceOwnership is retained as a compatibility alias
// while ownership is extracted from commands.
type ServiceOwnership = ownership.ServiceOwnership

// ServiceOwnershipStore is retained as a compatibility
// alias while ownership is extracted from commands.
type ServiceOwnershipStore = ownership.ServiceOwnershipStore

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

	// Installation requires a separate authorization path.
	// A lifecycle assignment never grants permission to
	// create or install a new service.
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

	if !ownership.ValidServiceID(command.ServiceID) {
		return fmt.Errorf(
			"%w: invalid service ID",
			ErrServiceNotAuthorized,
		)
	}

	assignment, err := a.store.Lookup(
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

	if !assignment.Enabled ||
		assignment.ServiceID != command.ServiceID ||
		assignment.NodeID != a.nodeID {
		return ErrServiceNotAuthorized
	}

	return nil
}
