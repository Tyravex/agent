package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func ValidateAPIListen(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = DefaultAPIListen
	}

	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", errors.New("api.listen must be a valid host:port address")
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("api.listen must use a loopback IP address")
	}

	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", errors.New("api.listen port must be between 1 and 65535")
	}

	return net.JoinHostPort(host, port), nil
}

func FromValues(
	panelURL string,
	nodeID string,
	nodeToken string,
	intervalValue string,
) (Config, error) {
	panelURL = strings.TrimSpace(panelURL)
	nodeID = strings.TrimSpace(nodeID)
	nodeToken = strings.TrimSpace(nodeToken)

	if panelURL == "" || nodeID == "" || nodeToken == "" {
		return Config{}, errors.New(
			"panel.url, node.id and node.token are required",
		)
	}

	parsed, err := url.Parse(panelURL)
	if err != nil {
		return Config{}, errors.New("invalid panel URL")
	}

	if (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
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
		interval, err = time.ParseDuration(
			strings.TrimSpace(intervalValue),
		)
		if err != nil {
			return Config{}, errors.New("invalid heartbeat interval")
		}

		if interval < 5*time.Second || interval > 60*time.Second {
			return Config{}, errors.New(
				"heartbeat interval must be between 5s and 60s",
			)
		}
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")

	return Config{
		PanelURL:          parsed.String(),
		NodeID:            nodeID,
		NodeToken:         nodeToken,
		HeartbeatInterval: interval,
		APIListen:         DefaultAPIListen,
	}, nil
}
