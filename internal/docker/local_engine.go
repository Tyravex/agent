package docker

import (
	"errors"
	"runtime"
	"strings"
)

var ErrLocalDockerEngineUnavailable = errors.New(
	"local Docker Engine endpoint is unavailable",
)

// LocalEngineHost resolves a supported local Docker Engine endpoint.
// It does not connect to Docker or inspect the process environment.
//
// An empty configured host selects the platform default.
// Explicit remote or nonstandard endpoints are rejected.
func LocalEngineHost(configuredHost string) (string, error) {
	var expected string

	switch runtime.GOOS {
	case "windows":
		expected = "npipe:////./pipe/docker_engine"
	case "linux":
		expected = "unix:///var/run/docker.sock"
	default:
		return "", ErrLocalDockerEngineUnavailable
	}

	configuredHost = strings.TrimSpace(configuredHost)

	if configuredHost != "" && configuredHost != expected {
		return "", ErrLocalDockerEngineUnavailable
	}

	return expected, nil
}
