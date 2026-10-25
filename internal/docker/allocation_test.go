package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func fakeAllocationRunner(
	containerIDs string,
	inspectOutput string,
) DockerCommandRunner {
	return func(
		ctx context.Context,
		args ...string,
	) ([]byte, error) {
		if len(args) >= 3 &&
			args[0] == "container" &&
			args[1] == "ls" {
			return []byte(containerIDs), nil
		}

		if len(args) >= 4 &&
			args[0] == "container" &&
			args[1] == "inspect" {
			return []byte(inspectOutput), nil
		}

		return nil, fmt.Errorf(
			"unexpected Docker command: %v",
			args,
		)
	}
}

func TestPortAllocationValidation(t *testing.T) {
	tests := []struct {
		name       string
		allocation PortAllocation
		valid      bool
	}{
		{
			name: "valid",
			allocation: PortAllocation{
				HostPort:      25566,
				ContainerPort: 25565,
			},
			valid: true,
		},
		{
			name: "missing host port",
			allocation: PortAllocation{
				ContainerPort: 25565,
			},
		},
		{
			name: "missing container port",
			allocation: PortAllocation{
				HostPort: 25566,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.allocation.Validate()

			if tt.valid && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !tt.valid &&
				!errors.Is(err, ErrInvalidAllocation) {
				t.Fatalf(
					"expected invalid allocation, got %v",
					err,
				)
			}
		})
	}
}

func TestPortAllocationAvailable(t *testing.T) {
	checker := NewPortAllocationChecker(
		fakeAllocationRunner(
			"container-one\n",
			`{"25565/tcp":[{"HostIp":"0.0.0.0","HostPort":"25565"}]}`,
		),
	)

	err := checker.CheckAvailable(
		context.Background(),
		PortAllocation{
			HostPort:      25566,
			ContainerPort: 25565,
		},
	)

	if err != nil {
		t.Fatalf("expected available port: %v", err)
	}
}

func TestPortAllocationConflict(t *testing.T) {
	checker := NewPortAllocationChecker(
		fakeAllocationRunner(
			"container-one\n",
			`{"25565/tcp":[{"HostIp":"0.0.0.0","HostPort":"25565"}]}`,
		),
	)

	err := checker.CheckAvailable(
		context.Background(),
		PortAllocation{
			HostPort:      25565,
			ContainerPort: 25565,
		},
	)

	if !errors.Is(err, ErrPortAllocated) {
		t.Fatalf(
			"expected allocated port, got %v",
			err,
		)
	}
}

func TestPortAllocationIgnoresUDP(t *testing.T) {
	checker := NewPortAllocationChecker(
		fakeAllocationRunner(
			"container-one\n",
			`{"25565/udp":[{"HostIp":"0.0.0.0","HostPort":"25565"}]}`,
		),
	)

	err := checker.CheckAvailable(
		context.Background(),
		PortAllocation{
			HostPort:      25565,
			ContainerPort: 25565,
		},
	)

	if err != nil {
		t.Fatalf("UDP must not block TCP: %v", err)
	}
}

func TestPortAllocationNoContainers(t *testing.T) {
	checker := NewPortAllocationChecker(
		fakeAllocationRunner("", ""),
	)

	err := checker.CheckAvailable(
		context.Background(),
		PortAllocation{
			HostPort:      25566,
			ContainerPort: 25565,
		},
	)

	if err != nil {
		t.Fatalf("expected available port: %v", err)
	}
}

func TestPortAllocationDetectsPortRange(t *testing.T) {
	checker := NewPortAllocationChecker(
		fakeAllocationRunner(
			"container-one\n",
			`{"25565/tcp":[{"HostIp":"","HostPort":"25560-25570"}]}`,
		),
	)

	err := checker.CheckAvailable(
		context.Background(),
		PortAllocation{
			HostPort:      25565,
			ContainerPort: 25565,
		},
	)

	if !errors.Is(err, ErrPortAllocated) {
		t.Fatalf(
			"expected range conflict, got %v",
			err,
		)
	}
}

func TestPortAllocationRejectsMalformedInspect(t *testing.T) {
	checker := NewPortAllocationChecker(
		fakeAllocationRunner(
			"container-one\n",
			"not-json",
		),
	)

	err := checker.CheckAvailable(
		context.Background(),
		PortAllocation{
			HostPort:      25566,
			ContainerPort: 25565,
		},
	)

	if !errors.Is(err, ErrDockerUnavailable) {
		t.Fatalf(
			"expected Docker inspection error, got %v",
			err,
		)
	}
}

func TestPortAllocationRejectsDockerFailure(t *testing.T) {
	checker := NewPortAllocationChecker(
		func(
			ctx context.Context,
			args ...string,
		) ([]byte, error) {
			return nil, errors.New("Docker offline")
		},
	)

	err := checker.CheckAvailable(
		context.Background(),
		PortAllocation{
			HostPort:      25566,
			ContainerPort: 25565,
		},
	)

	if !errors.Is(err, ErrDockerUnavailable) {
		t.Fatalf(
			"expected Docker unavailable, got %v",
			err,
		)
	}
}

func TestDiskQuotaNotFalselyAdvertised(t *testing.T) {
	capabilities := DetectDiskQuotaCapabilities()

	if capabilities.Enforced {
		t.Fatal(
			"named-volume disk quota must not be advertised",
		)
	}

	if !strings.Contains(
		capabilities.Reason,
		"not implemented",
	) {
		t.Fatalf(
			"unexpected capability reason: %q",
			capabilities.Reason,
		)
	}
}

func TestPortAllocationChecksStoppedContainers(t *testing.T) {
	var sawAll bool

	checker := NewPortAllocationChecker(
		func(
			ctx context.Context,
			args ...string,
		) ([]byte, error) {
			if len(args) >= 3 &&
				args[0] == "container" &&
				args[1] == "ls" {
				for _, arg := range args {
					if arg == "--all" {
						sawAll = true
					}
				}

				return []byte("stopped-container\n"), nil
			}

			return []byte(
				`{"25565/tcp":[{"HostIp":"","HostPort":"25565"}]}`,
			), nil
		},
	)

	err := checker.CheckAvailable(
		context.Background(),
		PortAllocation{
			HostPort:      25565,
			ContainerPort: 25565,
		},
	)

	if !sawAll {
		t.Fatal("Docker listing must include stopped containers")
	}

	if !errors.Is(err, ErrPortAllocated) {
		t.Fatalf(
			"expected stopped container conflict, got %v",
			err,
		)
	}
}
