package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	PanelURL          string
	NodeID            string
	NodeToken         string
	HeartbeatInterval time.Duration
}

func Load() (Config, error) {
	return FromValues(
		os.Getenv("TYRAVEX_PANEL_URL"),
		os.Getenv("TYRAVEX_NODE_ID"),
		os.Getenv("TYRAVEX_NODE_TOKEN"),
		os.Getenv("TYRAVEX_HEARTBEAT_INTERVAL_MS"),
	)
}

func FromValues(panelURL, nodeID, nodeToken, intervalValue string) (Config, error) {
	panelURL = strings.TrimSpace(panelURL)
	nodeID = strings.TrimSpace(nodeID)
	nodeToken = strings.TrimSpace(nodeToken)

	if panelURL == "" || nodeID == "" || nodeToken == "" {
		return Config{}, errors.New(
			"TYRAVEX_PANEL_URL, TYRAVEX_NODE_ID and TYRAVEX_NODE_TOKEN are required",
		)
	}

	parsed, err := url.Parse(panelURL)
	if err != nil {
		return Config{}, fmt.Errorf("invalid panel URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Config{}, errors.New("panel URL must use HTTP or HTTPS")
	}

	if parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return Config{}, errors.New("invalid panel URL")
	}

	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		ip := net.ParseIP(host)

		if !strings.EqualFold(host, "localhost") &&
			(ip == nil || !ip.IsLoopback()) {
			return Config{}, errors.New(
				"HTTPS is required for non-local panel connections",
			)
		}
	}

	interval := 30 * time.Second

	if strings.TrimSpace(intervalValue) != "" {
		ms, err := strconv.Atoi(strings.TrimSpace(intervalValue))
		if err != nil || ms < 5000 || ms > 60000 {
			return Config{}, errors.New(
				"TYRAVEX_HEARTBEAT_INTERVAL_MS must be between 5000 and 60000",
			)
		}

		interval = time.Duration(ms) * time.Millisecond
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")

	return Config{
		PanelURL:          parsed.String(),
		NodeID:            nodeID,
		NodeToken:         nodeToken,
		HeartbeatInterval: interval,
	}, nil
}
