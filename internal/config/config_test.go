package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidLocalConfig(t *testing.T) {
	cfg, err := FromValues(
		"http://localhost:3000",
		"node-123",
		"secret-token",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.HeartbeatInterval != 30*time.Second {
		t.Fatalf("unexpected interval: %v", cfg.HeartbeatInterval)
	}
}

func TestRejectRemoteHTTP(t *testing.T) {
	_, err := FromValues(
		"http://example.com",
		"node-123",
		"secret-token",
		"",
	)

	if err == nil {
		t.Fatal("expected HTTP rejection")
	}
}

func TestAcceptRemoteHTTPS(t *testing.T) {
	_, err := FromValues(
		"https://panel.example.com",
		"node-123",
		"secret-token",
		"15000",
	)

	if err != nil {
		t.Fatal(err)
	}
}

func TestRejectInvalidInterval(t *testing.T) {
	_, err := FromValues(
		"http://localhost:3000",
		"node-123",
		"secret-token",
		"1000",
	)

	if err == nil {
		t.Fatal("expected interval validation error")
	}
}

func TestRejectMissingCredentials(t *testing.T) {
	_, err := FromValues(
		"http://localhost:3000",
		"",
		"",
		"",
	)

	if err == nil {
		t.Fatal("expected missing credentials error")
	}
}

func TestRejectURLCredentials(t *testing.T) {
	_, err := FromValues(
		"https://user:password@example.com",
		"node-123",
		"secret-token",
		"",
	)

	if err == nil {
		t.Fatal("expected URL credentials rejection")
	}
}

func TestRejectUnsupportedScheme(t *testing.T) {
	_, err := FromValues(
		"file:///tmp/panel",
		"node-123",
		"secret-token",
		"",
	)

	if err == nil {
		t.Fatal("expected scheme rejection")
	}
}

func TestMissingErrorDoesNotExposeToken(t *testing.T) {
	_, err := FromValues(
		"",
		"node-123",
		"super-secret",
		"",
	)

	if err == nil || strings.Contains(err.Error(), "super-secret") {
		t.Fatal("invalid error handling")
	}
}
