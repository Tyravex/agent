package docker

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func supportedLocalDockerHost(t *testing.T) string {
	t.Helper()

	switch runtime.GOOS {
	case "windows":
		return "npipe:////./pipe/docker_engine"
	case "linux":
		return "unix:///var/run/docker.sock"
	default:
		t.Skip("unsupported operating system")
		return ""
	}
}

func TestNewRuntimeWithHost(t *testing.T) {
	host := supportedLocalDockerHost(t)

	instance, err := NewRuntimeWithHost(host)
	if err != nil {
		t.Fatal(err)
	}

	if instance.host != host {
		t.Fatalf("unexpected Docker host: %q", instance.host)
	}
}

func TestNewRuntimeWithHostRejectsUnsafeEndpoints(t *testing.T) {
	hosts := []string{
		"",
		"tcp://127.0.0.1:2375",
		"tcp://docker.example:2376",
		"unix:///tmp/docker.sock",
		"npipe:////./pipe/untrusted",
	}

	for _, host := range hosts {
		t.Run(host, func(t *testing.T) {
			instance, err := NewRuntimeWithHost(host)
			if err == nil || instance != nil {
				t.Fatalf("expected rejection for %q", host)
			}
		})
	}
}

func TestFilteredDockerEnvironment(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")
	t.Setenv("DOCKER_CONTEXT", "remote")
	t.Setenv("DOCKER_TLS_VERIFY", "1")
	t.Setenv("DOCKER_CERT_PATH", "/tmp/certs")
	t.Setenv("DOCKER_CONFIG", "/tmp/docker-config")

	for _, entry := range filteredDockerEnvironment() {
		key, _, _ := strings.Cut(entry, "=")

		if strings.HasPrefix(strings.ToUpper(key), "DOCKER_") {
			t.Fatalf("Docker environment variable leaked: %s", key)
		}
	}
}

func TestRuntimeWithHostPassesExplicitHost(t *testing.T) {
	host := supportedLocalDockerHost(t)

	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")
	t.Setenv("DOCKER_CONTEXT", "remote")
	t.Setenv("DOCKER_TLS_VERIFY", "1")
	t.Setenv("DOCKER_CERT_PATH", "/tmp/certs")
	t.Setenv("DOCKER_CONFIG", "/tmp/docker-config")

	instance, err := NewRuntimeWithHost(host)
	if err != nil {
		t.Fatal(err)
	}

	command := instance.newCommand(
		context.Background(),
		"container",
		"inspect",
		"tyravex-test",
	)

	expected := []string{
		"docker",
		"--host",
		host,
		"container",
		"inspect",
		"tyravex-test",
	}

	if len(command.Args) != len(expected) {
		t.Fatalf("unexpected command arguments: %q", command.Args)
	}

	for i, want := range expected {
		if command.Args[i] != want {
			t.Fatalf(
				"argument %d: got %q, want %q",
				i,
				command.Args[i],
				want,
			)
		}
	}

	if command.Env == nil {
		t.Fatal("expected explicitly configured process environment")
	}

	for _, entry := range command.Env {
		key, _, _ := strings.Cut(entry, "=")

		if strings.HasPrefix(strings.ToUpper(key), "DOCKER_") {
			t.Fatalf("Docker environment variable leaked: %s", key)
		}
	}
}

func TestRuntimeWithoutHostPreservesLegacyBehavior(t *testing.T) {
	instance := NewRuntime()

	command := instance.newCommand(
		context.Background(),
		"version",
	)

	if len(command.Args) != 2 ||
		command.Args[0] != "docker" ||
		command.Args[1] != "version" {
		t.Fatalf("unexpected legacy command arguments: %q", command.Args)
	}

	if command.Env != nil {
		t.Fatal("legacy runtime must inherit process environment")
	}
}
