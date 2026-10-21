package docker

import (
	"testing"
)

func TestNewClientDefault(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Fatalf("create default Docker client: %v", err)
	}
	defer c.Close()
}

func TestNewClientInvalidHost(t *testing.T) {
	_, err := NewClientWithHost("invalid://docker")
	if err == nil {
		t.Fatal("expected invalid Docker host error")
	}
}

func TestNewClientWindowsPipe(t *testing.T) {
	c, err := NewClientWithHost(
		"npipe:////./pipe/docker_engine",
	)
	if err != nil {
		t.Fatalf("create Windows Docker client: %v", err)
	}
	defer c.Close()
}

func TestNewClientUnixSocket(t *testing.T) {
	c, err := NewClientWithHost(
		"unix:///var/run/docker.sock",
	)
	if err != nil {
		t.Fatalf("create Unix Docker client: %v", err)
	}
	defer c.Close()
}

func TestNewClientCustomTCPHost(t *testing.T) {
	c, err := NewClientWithHost(
		"tcp://127.0.0.1:2375",
	)
	if err != nil {
		t.Fatalf("create TCP Docker client: %v", err)
	}
	defer c.Close()
}

func TestNewClientMissingTCPPort(t *testing.T) {
	_, err := NewClientWithHost("tcp://127.0.0.1")
	if err == nil {
		t.Fatal("expected missing TCP port error")
	}
}

func TestNewClientMissingSocketPath(t *testing.T) {
	_, err := NewClientWithHost("unix://")
	if err == nil {
		t.Fatal("expected missing socket path error")
	}
}
