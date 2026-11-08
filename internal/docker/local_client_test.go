package docker

import (
	"errors"
	"testing"
)

func TestNewLocalClient(t *testing.T) {
	host, err := LocalEngineHost("")
	if err != nil {
		if errors.Is(err, ErrLocalDockerEngineUnavailable) {
			t.Skip("unsupported operating system")
		}
		t.Fatal(err)
	}

	api, err := NewLocalClient("")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	if api.api == nil {
		t.Fatal("expected Docker SDK client")
	}

	if got := api.api.DaemonHost(); got != host {
		t.Fatalf("Docker host = %q, want %q", got, host)
	}
}

func TestNewLocalClientIgnoresDockerEnvironment(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")
	t.Setenv("DOCKER_CONTEXT", "remote")
	t.Setenv("DOCKER_TLS_VERIFY", "1")
	t.Setenv("DOCKER_CERT_PATH", "nonexistent")

	host, err := LocalEngineHost("")
	if err != nil {
		if errors.Is(err, ErrLocalDockerEngineUnavailable) {
			t.Skip("unsupported operating system")
		}
		t.Fatal(err)
	}

	api, err := NewLocalClient("")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()

	if got := api.api.DaemonHost(); got != host {
		t.Fatalf("Docker host = %q, want %q", got, host)
	}
}

func TestNewLocalClientRejectsUnsupportedEndpoints(t *testing.T) {
	hosts := []string{
		"tcp://127.0.0.1:2375",
		"tcp://docker.example:2376",
		"unix:///tmp/docker.sock",
		"npipe:////./pipe/untrusted",
	}

	for _, host := range hosts {
		t.Run(host, func(t *testing.T) {
			api, err := NewLocalClient(host)

			if !errors.Is(err, ErrLocalDockerEngineUnavailable) {
				t.Fatalf("expected unavailable error, got %v", err)
			}

			if api != nil {
				api.Close()
				t.Fatal("expected nil client")
			}
		})
	}
}
