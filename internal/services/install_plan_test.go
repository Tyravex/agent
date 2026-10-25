package services

import (
	"errors"
	"testing"
)

func validInstallRequest() InstallRequest {
	return InstallRequest{
		ServiceID:     "minecraft-1",
		Image:         "itzg/minecraft-server:latest",
		Startup:       "java -jar server.jar",
		MemoryMB:      2048,
		CPUPercent:    100,
		DiskMB:        10240,
		HostPort:      25566,
		ContainerPort: 25565,
		Variables: []InstallVariable{
			{
				Key:      "EULA",
				Value:    "TRUE",
				Required: true,
				HasValue: true,
			},
		},
	}
}

func TestBuildInstallPlanRejectsMissingPort(t *testing.T) {
	request := validInstallRequest()
	request.HostPort = 0

	_, err := BuildInstallPlan(
		request,
		InstallCapabilities{DiskQuotaEnforced: true},
	)

	if !errors.Is(err, ErrMissingPortAllocation) {
		t.Fatalf("expected missing port error, got %v", err)
	}
}

func TestBuildInstallPlanRejectsUnsupportedDiskQuota(t *testing.T) {
	_, err := BuildInstallPlan(
		validInstallRequest(),
		InstallCapabilities{DiskQuotaEnforced: false},
	)

	if !errors.Is(err, ErrDiskLimitUnsupported) {
		t.Fatalf("expected disk quota error, got %v", err)
	}
}

func TestBuildInstallPlanRejectsMissingRequiredValue(t *testing.T) {
	request := validInstallRequest()
	request.Variables[0].HasValue = false

	_, err := BuildInstallPlan(
		request,
		InstallCapabilities{DiskQuotaEnforced: true},
	)

	if !errors.Is(err, ErrMissingVariableValues) {
		t.Fatalf("expected required variable error, got %v", err)
	}
}

func TestBuildInstallPlanAcceptsCompleteRequest(t *testing.T) {
	request := validInstallRequest()

	plan, err := BuildInstallPlan(
		request,
		InstallCapabilities{DiskQuotaEnforced: true},
	)
	if err != nil {
		t.Fatalf("BuildInstallPlan: %v", err)
	}

	if plan.HostPort != 25566 ||
		plan.ContainerPort != 25565 {
		t.Fatalf("unexpected port allocation: %+v", plan)
	}

	if plan.Environment["EULA"] != "TRUE" {
		t.Fatal("missing EULA environment variable")
	}
}

func TestBuildInstallPlanRejectsDuplicateVariables(t *testing.T) {
	request := validInstallRequest()
	request.Variables = append(
		request.Variables,
		InstallVariable{
			Key:      "EULA",
			Value:    "FALSE",
			HasValue: true,
		},
	)

	_, err := BuildInstallPlan(
		request,
		InstallCapabilities{DiskQuotaEnforced: true},
	)
	if err == nil {
		t.Fatal("expected duplicate variable error")
	}
}

func TestBuildInstallPlanDoesNotExposeOptionalMissingValues(t *testing.T) {
	request := validInstallRequest()
	request.Variables = append(
		request.Variables,
		InstallVariable{
			Key:      "OPTIONAL",
			Required: false,
			HasValue: false,
		},
	)

	plan, err := BuildInstallPlan(
		request,
		InstallCapabilities{DiskQuotaEnforced: true},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, exists := plan.Environment["OPTIONAL"]; exists {
		t.Fatal("missing optional value must not be invented")
	}
}
