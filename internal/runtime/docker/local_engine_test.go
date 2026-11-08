package docker

import (
	"errors"
	"runtime"
	"testing"
)

func TestLocalEngineHost(t *testing.T) {
	var expected string

	switch runtime.GOOS {
	case "windows":
		expected = "npipe:////./pipe/docker_engine"
	case "linux":
		expected = "unix:///var/run/docker.sock"
	default:
		t.Skip("unsupported operating system")
	}

	tests := []struct {
		name string
		host string
	}{
		{name: "platform default", host: ""},
		{name: "explicit local endpoint", host: expected},
		{name: "trimmed local endpoint", host: " " + expected + " "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LocalEngineHost(tc.host)
			if err != nil {
				t.Fatal(err)
			}
			if got != expected {
				t.Fatalf("got %q, want %q", got, expected)
			}
		})
	}
}

func TestLocalEngineHostRejectsUnsupportedEndpoints(t *testing.T) {
	hosts := []string{
		"tcp://127.0.0.1:2375",
		"tcp://docker.example:2376",
		"unix:///tmp/docker.sock",
		"npipe:////./pipe/untrusted",
		"invalid://docker",
	}

	for _, host := range hosts {
		t.Run(host, func(t *testing.T) {
			got, err := LocalEngineHost(host)
			if !errors.Is(err, ErrLocalDockerEngineUnavailable) {
				t.Fatalf("expected unavailable error, got %v", err)
			}
			if got != "" {
				t.Fatalf("expected empty endpoint, got %q", got)
			}
		})
	}
}

func TestLocalEngineHostIgnoresDockerEnvironment(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")
	t.Setenv("DOCKER_CONTEXT", "remote")

	got, err := LocalEngineHost("")
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		if !errors.Is(err, ErrLocalDockerEngineUnavailable) {
			t.Fatalf("expected unavailable error, got %v", err)
		}
		return
	}

	if err != nil {
		t.Fatal(err)
	}

	if got == "" || got == "tcp://127.0.0.1:2375" {
		t.Fatalf("unexpected Docker endpoint: %q", got)
	}
}
