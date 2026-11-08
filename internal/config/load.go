package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

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
