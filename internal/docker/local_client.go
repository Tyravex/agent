package docker

import (
	"fmt"

	"github.com/moby/moby/client"
)

// NewLocalClient creates a Docker SDK client bound to the
// supported local Docker Engine endpoint.
//
// It does not read Docker-specific environment configuration
// through client.FromEnv and does not connect to the Engine.
func NewLocalClient(configuredHost string) (*Client, error) {
	host, err := LocalEngineHost(configuredHost)
	if err != nil {
		return nil, err
	}

	api, err := client.New(
		client.WithHost(host),
	)
	if err != nil {
		return nil, fmt.Errorf("create local Docker client: %w", err)
	}

	return &Client{api: api}, nil
}
