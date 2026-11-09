package app

import (
	"errors"
	"net/url"
	"strings"

	"github.com/Tyravex/agent/internal/command"
	"github.com/Tyravex/agent/internal/config"
	transport "github.com/Tyravex/agent/internal/transport/panelws"
)

var ErrRunnerOwnershipExecutionUnavailable = errors.New(
	"runner ownership execution is unavailable",
)

// prepareOwnershipExecution validates configuration and prepares
// ownership-bound lifecycle execution.
//
// This function does not connect to the Panel or Docker Engine.
func prepareOwnershipExecution(
	cfg config.Config,
	runtime commands.DockerServiceRuntime,
) (*transport.OwnershipExecution, error) {
	if !cfg.DockerEnabled ||
		strings.TrimSpace(cfg.NodeID) == "" ||
		strings.TrimSpace(cfg.NodeToken) == "" ||
		runtime == nil {
		return nil, ErrRunnerOwnershipExecutionUnavailable
	}

	parsed, err := url.Parse(cfg.PanelURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, ErrRunnerOwnershipExecutionUnavailable
	}

	// Docker CLI runtime does not currently honor cfg.DockerHost.
	// Reject explicit hosts until runtime host selection is unified.
	if strings.TrimSpace(cfg.DockerHost) != "" {
		return nil, ErrRunnerOwnershipExecutionUnavailable
	}

	syncManager := commands.NewOwnershipSyncManager(cfg.NodeID)

	return transport.NewOwnershipExecution(
		cfg.NodeID,
		syncManager,
		runtime,
	)
}
