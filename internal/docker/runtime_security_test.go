package docker

import (
	"slices"
	"testing"
)

func TestContainerCreateSecurity(t *testing.T) {
	spec := ServerSpec{
		ID:            "mc-test",
		Image:         "itzg/minecraft-server:latest",
		MemoryMB:      2048,
		CPUPercent:    100,
		Port:          25565,
		ContainerPort: 25565,
		Env: map[string]string{
			"EULA": "TRUE",
			"TYPE": "PAPER",
		},
	}

	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}

	args := containerCreateArgs(spec)

	required := [][]string{
		{"--cap-drop", "ALL"},
		{"--cap-add", "SETUID"},
		{"--cap-add", "SETGID"},
		{"--security-opt", "no-new-privileges:true"},
		{"--memory", "2048m"},
		{"--cpus", "1.00"},
		{"--publish", "25565:25565/tcp"},
		{"--mount", "type=volume,source=tyravex-data-mc-test,target=/data"},
	}

	for _, pair := range required {
		if !containsPair(args, pair[0], pair[1]) {
			t.Errorf("missing Docker option %q %q", pair[0], pair[1])
		}
	}

	if !slices.Contains(args, "itzg/minecraft-server:latest") {
		t.Error("missing image")
	}

	if args[len(args)-1] != spec.Image {
		t.Error("image must be the last argument")
	}
}

func TestContainerCreateEnvironmentOrder(t *testing.T) {
	spec := ServerSpec{
		ID:            "test",
		Image:         "example/image:1",
		MemoryMB:      1024,
		CPUPercent:    50,
		Port:          25565,
		ContainerPort: 25565,
		Env: map[string]string{
			"Z_LAST":  "z",
			"A_FIRST": "a",
		},
	}

	args := containerCreateArgs(spec)

	first := indexPair(args, "--env", "A_FIRST=a")
	last := indexPair(args, "--env", "Z_LAST=z")

	if first < 0 || last < 0 || first >= last {
		t.Fatalf("environment variables are not sorted: %v", args)
	}
}

func TestContainerCreateDoesNotAddPrivileged(t *testing.T) {
	spec := ServerSpec{
		ID:            "test",
		Image:         "example/image:1",
		MemoryMB:      1024,
		CPUPercent:    50,
		Port:          25565,
		ContainerPort: 25565,
	}

	args := containerCreateArgs(spec)

	if slices.Contains(args, "--privileged") {
		t.Fatal("container must not be privileged")
	}

	if slices.Contains(args, "SYS_ADMIN") {
		t.Fatal("SYS_ADMIN capability must not be added")
	}
}

func containsPair(args []string, key, value string) bool {
	return indexPair(args, key, value) >= 0
}

func indexPair(args []string, key, value string) int {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return i
		}
	}
	return -1
}
