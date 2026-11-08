package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

var (
	ErrInvalidAllocation = errors.New(
		"invalid Docker port allocation",
	)
	ErrPortAllocated = errors.New(
		"Docker host port is already allocated",
	)
	ErrDockerUnavailable = errors.New(
		"Docker allocation inspection failed",
	)
)

// PortAllocation describes an explicit TCP port mapping.
//
// HostPort must be assigned by the Panel. This package does not
// select ports or silently substitute default values.
type PortAllocation struct {
	HostPort      uint16
	ContainerPort uint16
}

func (a PortAllocation) Validate() error {
	if a.HostPort == 0 || a.ContainerPort == 0 {
		return ErrInvalidAllocation
	}

	return nil
}

// DockerCommandRunner executes a Docker CLI command.
//
// Keeping this dependency injectable makes allocation checks
// testable without Docker Desktop or Docker Engine.
type DockerCommandRunner func(
	ctx context.Context,
	args ...string,
) ([]byte, error)

func DefaultDockerCommandRunner(
	ctx context.Context,
	args ...string,
) ([]byte, error) {
	output, err := exec.CommandContext(
		ctx,
		"docker",
		args...,
	).CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf(
			"docker command failed: %w: %s",
			err,
			strings.TrimSpace(string(output)),
		)
	}

	return output, nil
}

type PortAllocationChecker struct {
	run DockerCommandRunner
}

func NewPortAllocationChecker(
	run DockerCommandRunner,
) *PortAllocationChecker {
	if run == nil {
		run = DefaultDockerCommandRunner
	}

	return &PortAllocationChecker{run: run}
}

// CheckAvailable inspects configured port bindings of all Docker
// containers, including stopped containers.
//
// A successful check is advisory: another process may allocate
// the port immediately afterwards. Docker's container creation
// and startup remain the final enforcement points.
//
// The check is intentionally conservative: any matching TCP
// host port is treated as a conflict, regardless of HostIp.
func (c *PortAllocationChecker) CheckAvailable(
	ctx context.Context,
	allocation PortAllocation,
) error {
	if err := allocation.Validate(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	output, err := c.run(
		ctx,
		"container",
		"ls",
		"--all",
		"--quiet",
	)
	if err != nil {
		return fmt.Errorf(
			"%w: list containers: %v",
			ErrDockerUnavailable,
			err,
		)
	}

	ids := strings.Fields(string(output))
	if len(ids) == 0 {
		return nil
	}

	args := append(
		[]string{
			"container",
			"inspect",
			"--format",
			"{{json .HostConfig.PortBindings}}",
		},
		ids...,
	)

	output, err = c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf(
			"%w: inspect port bindings: %v",
			ErrDockerUnavailable,
			err,
		)
	}

	bindings, err := decodeContainerPortBindings(output)
	if err != nil {
		return fmt.Errorf(
			"%w: decode port bindings: %v",
			ErrDockerUnavailable,
			err,
		)
	}

	targetPort := strconv.FormatUint(
		uint64(allocation.HostPort),
		10,
	)

	for _, containerBindings := range bindings {
		for containerPort, hostBindings := range containerBindings {
			// TCP and UDP use independent port namespaces.
			if !strings.HasSuffix(
				strings.ToLower(containerPort),
				"/tcp",
			) {
				continue
			}

			for _, binding := range hostBindings {
				if binding.HostPort == targetPort {
					return fmt.Errorf(
						"%w: TCP %s",
						ErrPortAllocated,
						targetPort,
					)
				}

				// Docker may store a host port range.
				// Reject uncertain configurations instead
				// of incorrectly reporting availability.
				if portInRange(
					targetPort,
					binding.HostPort,
				) {
					return fmt.Errorf(
						"%w: TCP %s",
						ErrPortAllocated,
						targetPort,
					)
				}
			}
		}
	}

	return nil
}

type dockerHostPortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type dockerContainerPortBindings map[string][]dockerHostPortBinding

func decodeContainerPortBindings(
	output []byte,
) ([]dockerContainerPortBindings, error) {
	lines := strings.Split(
		strings.TrimSpace(string(output)),
		"\n",
	)

	result := make(
		[]dockerContainerPortBindings,
		0,
		len(lines),
	)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Docker can return null for containers without
		// configured host port bindings.
		if line == "null" {
			result = append(
				result,
				dockerContainerPortBindings{},
			)
			continue
		}

		var bindings dockerContainerPortBindings

		if err := json.Unmarshal(
			[]byte(line),
			&bindings,
		); err != nil {
			return nil, err
		}

		if bindings == nil {
			bindings = dockerContainerPortBindings{}
		}

		result = append(result, bindings)
	}

	return result, nil
}

func portInRange(
	target string,
	configured string,
) bool {
	parts := strings.Split(configured, "-")
	if len(parts) != 2 {
		return false
	}

	port, err := strconv.Atoi(target)
	if err != nil {
		return false
	}

	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}

	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}

	return start <= port && port <= end
}

// DiskQuotaCapabilities describes what the runtime can actually
// guarantee for persistent service data.
type DiskQuotaCapabilities struct {
	Enforced bool
	Reason   string
}

// DetectDiskQuotaCapabilities returns the current conservative
// capability for Tyravex's Docker named-volume runtime.
//
// Docker memory/CPU limits do not enforce a diskMb quota.
// Checking free disk space is not equivalent to a hard quota.
//
// This must remain false until a tested platform-specific
// quota implementation is integrated.
func DetectDiskQuotaCapabilities() DiskQuotaCapabilities {
	return DiskQuotaCapabilities{
		Enforced: false,
		Reason: "Docker named-volume disk quota enforcement " +
			"is not implemented",
	}
}
