package agent

import "github.com/Tyravex/agent/internal/config"

// ownershipIdentity captures configuration that must remain
// unchanged between local ownership preparation and activation.
//
// Do not log this structure: it contains the node token.
type ownershipIdentity struct {
	panelURL      string
	nodeID        string
	nodeToken     string
	dockerEnabled bool
	dockerHost    string
}

func ownershipIdentityFromConfig(cfg config.Config) ownershipIdentity {
	return ownershipIdentity{
		panelURL:      cfg.PanelURL,
		nodeID:        cfg.NodeID,
		nodeToken:     cfg.NodeToken,
		dockerEnabled: cfg.DockerEnabled,
		dockerHost:    cfg.DockerHost,
	}
}
