package config

import "time"

type Config struct {
	PanelURL          string
	NodeID            string
	NodeToken         string
	HeartbeatInterval time.Duration
	DockerEnabled     bool
	DockerHost        string
	APIListen         string
	APIToken          string
}
