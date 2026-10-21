package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const DefaultAPIListen = "127.0.0.1:8080"

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

type fileConfig struct {
	Panel struct {
		URL string `yaml:"url"`
	} `yaml:"panel"`

	Node struct {
		ID    string `yaml:"id"`
		Token string `yaml:"token"`
	} `yaml:"node"`

	Heartbeat struct {
		Interval string `yaml:"interval"`
	} `yaml:"heartbeat"`

	Docker struct {
		Enabled bool   `yaml:"enabled"`
		Host    string `yaml:"host"`
	} `yaml:"docker"`

	API struct {
		Listen string `yaml:"listen"`
		Token  string `yaml:"token"`
	} `yaml:"api"`
}

func Load() (Config, error) {
	return LoadFile("config.yml")
}

func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read configuration: %w", err)
	}

	var raw fileConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(&raw); err != nil {
		return Config{}, fmt.Errorf("parse configuration: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Config{}, errors.New("configuration must contain one YAML document")
	}

	cfg, err := FromValues(
		raw.Panel.URL,
		raw.Node.ID,
		raw.Node.Token,
		raw.Heartbeat.Interval,
	)
	if err != nil {
		return Config{}, err
	}

	cfg.DockerEnabled = raw.Docker.Enabled
	cfg.DockerHost = strings.TrimSpace(raw.Docker.Host)

	cfg.APIListen, err = ValidateAPIListen(raw.API.Listen)
	if err != nil {
		return Config{}, err
	}

	cfg.APIToken = strings.TrimSpace(raw.API.Token)
	if len(cfg.APIToken) < 32 {
		return Config{}, errors.New("api.token must contain at least 32 characters")
	}
	if cfg.APIToken == cfg.NodeToken {
		return Config{}, errors.New("api.token must differ from node.token")
	}

	return cfg, nil
}

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
