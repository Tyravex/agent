package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tyravex/agent/internal/docker"
)

var ErrDockerRuntimeUnavailable = errors.New(
	"Docker runtime unavailable",
)

// DockerServiceRuntime contains only the runtime operations
// required by the service command dispatcher.
//
// The interface is intentionally narrow to allow tests
// without Docker Desktop or Docker Engine.
type DockerServiceRuntime interface {
	Start(ctx context.Context, serviceID string) error
	Stop(ctx context.Context, serviceID string) error
	Restart(ctx context.Context, serviceID string) error
}

// DockerExecutor adapts the Docker runtime to commands.Executor.
//
// It does not install containers. Installation requires a
// validated InstallPlan and is handled separately.
type DockerExecutor struct {
	runtime DockerServiceRuntime
}

var _ Executor = (*DockerExecutor)(nil)
var _ DockerServiceRuntime = (*docker.Runtime)(nil)

func NewDockerExecutor(
	runtime DockerServiceRuntime,
) *DockerExecutor {
	return &DockerExecutor{
		runtime: runtime,
	}
}

func (e *DockerExecutor) Start(
	ctx context.Context,
	serviceID string,
) error {
	if err := e.validate(ctx, serviceID); err != nil {
		return err
	}

	if err := e.runtime.Start(ctx, serviceID); err != nil {
		return fmt.Errorf(
			"start service %q: %w",
			serviceID,
			err,
		)
	}

	return nil
}

func (e *DockerExecutor) Stop(
	ctx context.Context,
	serviceID string,
) error {
	if err := e.validate(ctx, serviceID); err != nil {
		return err
	}

	if err := e.runtime.Stop(ctx, serviceID); err != nil {
		return fmt.Errorf(
			"stop service %q: %w",
			serviceID,
			err,
		)
	}

	return nil
}

func (e *DockerExecutor) Restart(
	ctx context.Context,
	serviceID string,
) error {
	if err := e.validate(ctx, serviceID); err != nil {
		return err
	}

	if err := e.runtime.Restart(ctx, serviceID); err != nil {
		return fmt.Errorf(
			"restart service %q: %w",
			serviceID,
			err,
		)
	}

	return nil
}

func (e *DockerExecutor) validate(
	ctx context.Context,
	serviceID string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if strings.TrimSpace(serviceID) == "" {
		return fmt.Errorf(
			"%w: missing service ID",
			ErrInvalidCommand,
		)
	}

	if e == nil || e.runtime == nil {
		return ErrDockerRuntimeUnavailable
	}

	return nil
}
