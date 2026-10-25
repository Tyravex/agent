package transport

import (
	"errors"
	"fmt"

	"github.com/Tyravex/agent/internal/services"
)

// ErrInstallAllocationUnavailable means the current Panel protocol
// does not provide the network allocation required by Docker.
var ErrInstallAllocationUnavailable = errors.New(
	"service installation has no assigned network port",
)

// ErrInstallVariablesUnavailable means the Panel supplies variable
// metadata but not the values required for execution.
var ErrInstallVariablesUnavailable = errors.New(
	"service installation has no variable values",
)

// InstallAllocation represents a trusted port assignment resolved
// independently of the current version-1 installation payload.
//
// The caller must obtain these values from an authoritative source.
// They must never be generated from a default Minecraft port.
type InstallAllocation struct {
	HostPort      uint16
	ContainerPort uint16
}

// InstallInputs contains installation data that is not present in
// the current version-1 Panel protocol.
//
// This type is deliberately separate from ServiceInstallPayload
// so existing wire-format validation remains unchanged.
type InstallInputs struct {
	Allocation InstallAllocation
	Variables  map[string]string
}

// BuildServiceInstallPlan converts a validated Panel command into
// an installation plan.
//
// It does not invoke Docker, create volumes, or change any service
// state. Missing inputs cause a deterministic error.
func BuildServiceInstallPlan(
	command ServiceInstallCommand,
	inputs InstallInputs,
	capabilities services.InstallCapabilities,
) (services.InstallPlan, error) {
	if err := ValidateServiceInstallPayload(command.Install); err != nil {
		return services.InstallPlan{}, fmt.Errorf(
			"invalid installation payload: %w",
			err,
		)
	}

	if !validInstallCommandID(command.CommandID) {
		return services.InstallPlan{}, errors.New(
			"invalid installation command ID",
		)
	}

	if command.ServiceID != command.Install.ServiceID {
		return services.InstallPlan{}, errors.New(
			"installation service ID mismatch",
		)
	}

	if inputs.Allocation.HostPort == 0 ||
		inputs.Allocation.ContainerPort == 0 {
		return services.InstallPlan{}, ErrInstallAllocationUnavailable
	}

	variables := make(
		[]services.InstallVariable,
		0,
		len(command.Install.Variables),
	)

	allowed := make(
		map[string]struct{},
		len(command.Install.Variables),
	)

	for _, variable := range command.Install.Variables {
		allowed[variable.Key] = struct{}{}

		value, exists := inputs.Variables[variable.Key]

		if variable.Required && !exists {
			return services.InstallPlan{}, fmt.Errorf(
				"%w: %s",
				ErrInstallVariablesUnavailable,
				variable.Key,
			)
		}

		variables = append(
			variables,
			services.InstallVariable{
				Key:      variable.Key,
				Value:    value,
				Required: variable.Required,
				HasValue: exists,
			},
		)
	}

	// Never pass arbitrary additional environment variables into
	// a container. Only variables declared by the egg are allowed.
	for key := range inputs.Variables {
		if _, exists := allowed[key]; !exists {
			return services.InstallPlan{}, fmt.Errorf(
				"undeclared installation variable %q",
				key,
			)
		}
	}

	request := services.InstallRequest{
		ServiceID:     command.ServiceID,
		Image:         command.Install.Runtime.Image,
		Startup:       command.Install.Runtime.Startup,
		MemoryMB:      command.Install.Resources.MemoryMB,
		CPUPercent:    command.Install.Resources.CPUPercent,
		DiskMB:        command.Install.Resources.DiskMB,
		HostPort:      inputs.Allocation.HostPort,
		ContainerPort: inputs.Allocation.ContainerPort,
		Variables:     variables,
	}

	return services.BuildInstallPlan(
		request,
		capabilities,
	)
}
