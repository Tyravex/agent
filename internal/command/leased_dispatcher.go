package commands

import (
	"context"
	"fmt"

	"github.com/Tyravex/agent/internal/ownership"
)

// ErrLeaseUnavailable remains available to existing callers.
var ErrLeaseUnavailable = ownership.ErrLeaseUnavailable

// ServiceLeaseStore executes a callback while the trusted
// ownership assignment remains protected from changes.
//
// Implementations must keep the assignment stable until
// the callback returns.
// ServiceLeaseStore is retained as a compatibility alias
// while ownership is extracted from commands.
type ServiceLeaseStore = ownership.ServiceLeaseStore

// LeasedDispatcher executes lifecycle commands while
// holding an ownership lease.
//
// It never supports service.install.
type LeasedDispatcher struct {
	nodeID   string
	store    ServiceLeaseStore
	executor Executor
}

func NewLeasedDispatcher(
	nodeID string,
	store ServiceLeaseStore,
	executor Executor,
) *LeasedDispatcher {
	return &LeasedDispatcher{
		nodeID:   nodeID,
		store:    store,
		executor: executor,
	}
}

func (d *LeasedDispatcher) Execute(
	ctx context.Context,
	command Command,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := Validate(command); err != nil {
		return err
	}

	if d == nil ||
		d.nodeID == "" ||
		d.store == nil {
		return ErrLeaseUnavailable
	}

	if d.executor == nil {
		return ErrExecutorUnavailable
	}

	if !ownership.ValidServiceID(
		command.ServiceID,
	) {
		return ErrServiceNotAuthorized
	}

	// Installation is intentionally excluded from the
	// lifecycle authorization and execution path.
	switch command.Operation {
	case ServiceStart, ServiceStop, ServiceRestart:
	default:
		return ErrServiceNotAuthorized
	}

	return d.store.WithAuthorizedService(
		ctx,
		d.nodeID,
		command.ServiceID,
		func() error {
			if err := ctx.Err(); err != nil {
				return err
			}

			switch command.Operation {
			case ServiceStart:
				if err := d.executor.Start(
					ctx,
					command.ServiceID,
				); err != nil {
					return fmt.Errorf(
						"start service %q: %w",
						command.ServiceID,
						err,
					)
				}

			case ServiceStop:
				if err := d.executor.Stop(
					ctx,
					command.ServiceID,
				); err != nil {
					return fmt.Errorf(
						"stop service %q: %w",
						command.ServiceID,
						err,
					)
				}

			case ServiceRestart:
				if err := d.executor.Restart(
					ctx,
					command.ServiceID,
				); err != nil {
					return fmt.Errorf(
						"restart service %q: %w",
						command.ServiceID,
						err,
					)
				}
			}

			return nil
		},
	)
}

var _ interface {
	Execute(context.Context, Command) error
} = (*LeasedDispatcher)(nil)
