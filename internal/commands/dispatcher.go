package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Operation string

const (
	ServiceInstall Operation = "service.install"
	ServiceStart   Operation = "service.start"
	ServiceStop    Operation = "service.stop"
	ServiceRestart Operation = "service.restart"
)

var (
	ErrInvalidCommand       = errors.New("invalid command")
	ErrUnsupportedOperation = errors.New("unsupported operation")
	ErrExecutorUnavailable  = errors.New("command executor unavailable")
)

// Command is the internal representation of a Panel command.
//
// Parameters are not decoded here. The transport layer is
// responsible for strict protocol validation before dispatch.
type Command struct {
	ID        string
	ServiceID string
	Operation Operation
}

// Executor performs service operations.
//
// Implementations must honor context cancellation where possible.
// Install is deliberately separate because installation requires
// a validated plan, allocations, variables and storage policy.
type Executor interface {
	Start(ctx context.Context, serviceID string) error
	Stop(ctx context.Context, serviceID string) error
	Restart(ctx context.Context, serviceID string) error
}

// InstallExecutor is a separate capability.
//
// The dispatcher intentionally does not accept an incomplete
// service.install command. Installation will be connected once
// its full validated request is available.
type InstallExecutor interface {
	Install(ctx context.Context, command Command) error
}

type Dispatcher struct {
	executor  Executor
	installer InstallExecutor
}

func NewDispatcher(
	executor Executor,
	installer InstallExecutor,
) *Dispatcher {
	return &Dispatcher{
		executor:  executor,
		installer: installer,
	}
}

func (d *Dispatcher) Execute(
	ctx context.Context,
	command Command,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := Validate(command); err != nil {
		return err
	}

	switch command.Operation {
	case ServiceInstall:
		if d.installer == nil {
			return fmt.Errorf(
				"%w: installation",
				ErrExecutorUnavailable,
			)
		}
		return d.installer.Install(ctx, command)

	case ServiceStart:
		if d.executor == nil {
			return fmt.Errorf(
				"%w: start",
				ErrExecutorUnavailable,
			)
		}
		return d.executor.Start(ctx, command.ServiceID)

	case ServiceStop:
		if d.executor == nil {
			return fmt.Errorf(
				"%w: stop",
				ErrExecutorUnavailable,
			)
		}
		return d.executor.Stop(ctx, command.ServiceID)

	case ServiceRestart:
		if d.executor == nil {
			return fmt.Errorf(
				"%w: restart",
				ErrExecutorUnavailable,
			)
		}
		return d.executor.Restart(ctx, command.ServiceID)

	default:
		return fmt.Errorf(
			"%w: %s",
			ErrUnsupportedOperation,
			command.Operation,
		)
	}
}

func Validate(command Command) error {
	if strings.TrimSpace(command.ID) == "" {
		return fmt.Errorf(
			"%w: missing command ID",
			ErrInvalidCommand,
		)
	}

	if strings.TrimSpace(command.ServiceID) == "" {
		return fmt.Errorf(
			"%w: missing service ID",
			ErrInvalidCommand,
		)
	}

	switch command.Operation {
	case ServiceInstall, ServiceStart, ServiceStop, ServiceRestart:
		return nil

	default:
		return fmt.Errorf(
			"%w: %s",
			ErrUnsupportedOperation,
			command.Operation,
		)
	}
}
