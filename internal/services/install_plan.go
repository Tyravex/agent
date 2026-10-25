package services

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrMissingPortAllocation = errors.New(
		"installation requires an explicit port allocation",
	)
	ErrDiskLimitUnsupported = errors.New(
		"installation requires enforceable disk quota support",
	)
	ErrMissingVariableValues = errors.New(
		"installation requires values for mandatory variables",
	)
)

var installEnvKeyPattern = regexp.MustCompile(
	`^[A-Za-z_][A-Za-z0-9_]*$`,
)

// InstallRequest describes the information required to turn a
// validated Panel installation command into a Docker installation.
//
// This is an internal planning contract, not the current WebSocket
// payload. The transport layer must not invent missing fields.
type InstallRequest struct {
	ServiceID     string
	Image         string
	Startup       string
	MemoryMB      int64
	CPUPercent    int64
	DiskMB        int64
	HostPort      uint16
	ContainerPort uint16
	Variables     []InstallVariable
}

type InstallVariable struct {
	Key      string
	Value    string
	Required bool
	HasValue bool
}

type InstallCapabilities struct {
	DiskQuotaEnforced bool
}

type InstallPlan struct {
	ServiceID     string
	Image         string
	Startup       string
	MemoryMB      int64
	CPUPercent    int64
	DiskMB        int64
	HostPort      uint16
	ContainerPort uint16
	Environment   map[string]string
}

// BuildInstallPlan rejects incomplete installation requests before
// Docker is called. No containers or volumes are created here.
func BuildInstallPlan(
	request InstallRequest,
	capabilities InstallCapabilities,
) (InstallPlan, error) {
	if strings.TrimSpace(request.ServiceID) == "" {
		return InstallPlan{}, errors.New("missing service ID")
	}
	if strings.TrimSpace(request.Image) == "" {
		return InstallPlan{}, errors.New("missing Docker image")
	}
	if strings.TrimSpace(request.Startup) == "" {
		return InstallPlan{}, errors.New("missing startup command")
	}
	if request.MemoryMB < 128 {
		return InstallPlan{}, errors.New("memory limit is too low")
	}
	if request.CPUPercent < 1 || request.CPUPercent > 100 {
		return InstallPlan{}, errors.New("invalid CPU limit")
	}
	if request.DiskMB <= 0 {
		return InstallPlan{}, errors.New("invalid disk limit")
	}

	if request.HostPort == 0 || request.ContainerPort == 0 {
		return InstallPlan{}, ErrMissingPortAllocation
	}

	environment := make(map[string]string, len(request.Variables))

	for _, variable := range request.Variables {
		if !installEnvKeyPattern.MatchString(variable.Key) {
			return InstallPlan{}, fmt.Errorf(
				"invalid environment variable key %q",
				variable.Key,
			)
		}

		if _, exists := environment[variable.Key]; exists {
			return InstallPlan{}, fmt.Errorf(
				"duplicate environment variable %q",
				variable.Key,
			)
		}

		if variable.Required && !variable.HasValue {
			return InstallPlan{}, fmt.Errorf(
				"%w: %s",
				ErrMissingVariableValues,
				variable.Key,
			)
		}

		if !variable.HasValue {
			continue
		}

		if strings.ContainsRune(variable.Value, '\x00') {
			return InstallPlan{}, errors.New(
				"environment variable contains NUL",
			)
		}

		environment[variable.Key] = variable.Value
	}

	if !capabilities.DiskQuotaEnforced {
		return InstallPlan{}, ErrDiskLimitUnsupported
	}

	return InstallPlan{
		ServiceID:     request.ServiceID,
		Image:         request.Image,
		Startup:       request.Startup,
		MemoryMB:      request.MemoryMB,
		CPUPercent:    request.CPUPercent,
		DiskMB:        request.DiskMB,
		HostPort:      request.HostPort,
		ContainerPort: request.ContainerPort,
		Environment:   environment,
	}, nil
}
