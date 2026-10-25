package transport

import (
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/services"
)

func validServiceInstallPlanCommand() ServiceInstallCommand {
	return ServiceInstallCommand{
		CommandID: "command-123",
		ServiceID: "service-123",
		Install: ServiceInstallPayload{
			FormatVersion: 1,
			ServiceID:     "service-123",
			JobID:         "job-123",
			NodeID:        "node-123",
			Egg: ServiceInstallEgg{
				ID:      "egg-123",
				Key:     "minecraft-paper",
				Version: "1.0.0",
			},
			Runtime: ServiceInstallRuntime{
				Image:   "itzg/minecraft-server:latest",
				Startup: "java -jar server.jar",
			},
			Resources: ServiceInstallResources{
				MemoryMB:   2048,
				CPUPercent: 100,
				DiskMB:     10240,
			},
			Variables: []ServiceInstallVariable{
				{
					Key:      "EULA",
					Required: true,
				},
			},
		},
	}
}

func validServiceInstallInputs() InstallInputs {
	return InstallInputs{
		Allocation: InstallAllocation{
			HostPort:      25566,
			ContainerPort: 25565,
		},
		Variables: map[string]string{
			"EULA": "TRUE",
		},
	}
}

func supportedInstallCapabilities() services.InstallCapabilities {
	return services.InstallCapabilities{
		DiskQuotaEnforced: true,
	}
}

func TestServiceInstallPlanRejectsMissingAllocation(t *testing.T) {
	command := validServiceInstallPlanCommand()

	_, err := BuildServiceInstallPlan(
		command,
		InstallInputs{
			Variables: map[string]string{
				"EULA": "TRUE",
			},
		},
		supportedInstallCapabilities(),
	)

	if !errors.Is(err, ErrInstallAllocationUnavailable) {
		t.Fatalf(
			"expected missing allocation error, got %v",
			err,
		)
	}
}

func TestServiceInstallPlanRejectsMissingRequiredVariable(
	t *testing.T,
) {
	command := validServiceInstallPlanCommand()
	inputs := validServiceInstallInputs()
	inputs.Variables = nil

	_, err := BuildServiceInstallPlan(
		command,
		inputs,
		supportedInstallCapabilities(),
	)

	if !errors.Is(err, ErrInstallVariablesUnavailable) {
		t.Fatalf(
			"expected missing variable error, got %v",
			err,
		)
	}
}

func TestServiceInstallPlanRejectsUnsupportedDiskQuota(
	t *testing.T,
) {
	_, err := BuildServiceInstallPlan(
		validServiceInstallPlanCommand(),
		validServiceInstallInputs(),
		services.InstallCapabilities{
			DiskQuotaEnforced: false,
		},
	)

	if !errors.Is(err, services.ErrDiskLimitUnsupported) {
		t.Fatalf(
			"expected unsupported disk quota error, got %v",
			err,
		)
	}
}

func TestServiceInstallPlanAcceptsCompleteInputs(t *testing.T) {
	plan, err := BuildServiceInstallPlan(
		validServiceInstallPlanCommand(),
		validServiceInstallInputs(),
		supportedInstallCapabilities(),
	)
	if err != nil {
		t.Fatalf("BuildServiceInstallPlan: %v", err)
	}

	if plan.ServiceID != "service-123" {
		t.Fatalf("unexpected service ID: %q", plan.ServiceID)
	}

	if plan.HostPort != 25566 {
		t.Fatalf("unexpected host port: %d", plan.HostPort)
	}

	if plan.ContainerPort != 25565 {
		t.Fatalf(
			"unexpected container port: %d",
			plan.ContainerPort,
		)
	}

	if plan.Environment["EULA"] != "TRUE" {
		t.Fatal("EULA variable was not transferred")
	}
}

func TestServiceInstallPlanRejectsUndeclaredVariable(t *testing.T) {
	inputs := validServiceInstallInputs()
	inputs.Variables["UNEXPECTED"] = "value"

	_, err := BuildServiceInstallPlan(
		validServiceInstallPlanCommand(),
		inputs,
		supportedInstallCapabilities(),
	)

	if err == nil {
		t.Fatal("expected undeclared variable rejection")
	}
}

func TestServiceInstallPlanRejectsServiceIDMismatch(t *testing.T) {
	command := validServiceInstallPlanCommand()
	command.ServiceID = "different-service"

	_, err := BuildServiceInstallPlan(
		command,
		validServiceInstallInputs(),
		supportedInstallCapabilities(),
	)

	if err == nil {
		t.Fatal("expected service ID mismatch")
	}
}

func TestServiceInstallPlanPreservesEmptyVariableValue(
	t *testing.T,
) {
	inputs := validServiceInstallInputs()
	inputs.Variables["EULA"] = ""

	plan, err := BuildServiceInstallPlan(
		validServiceInstallPlanCommand(),
		inputs,
		supportedInstallCapabilities(),
	)
	if err != nil {
		t.Fatalf("BuildServiceInstallPlan: %v", err)
	}

	value, exists := plan.Environment["EULA"]
	if !exists || value != "" {
		t.Fatal(
			"explicit empty value must remain distinguishable from missing value",
		)
	}
}

func TestServiceInstallPlanRejectsInvalidPayload(t *testing.T) {
	command := validServiceInstallPlanCommand()
	command.Install.FormatVersion = 99

	_, err := BuildServiceInstallPlan(
		command,
		validServiceInstallInputs(),
		supportedInstallCapabilities(),
	)

	if err == nil {
		t.Fatal("expected invalid payload rejection")
	}
}
