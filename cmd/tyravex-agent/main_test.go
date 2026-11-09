package main

import (
	"context"
	"net"
	"testing"
	"time"

	agent "github.com/Tyravex/agent/internal/app"
	"github.com/Tyravex/agent/internal/config"
)

func TestRunAgentRejectsOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	cfg := config.Config{
		APIListen: listener.Addr().String(),
		APIToken:  "test-api-token",
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Second,
	)
	defer cancel()

	if err := agent.Run(ctx, cfg); err == nil {
		t.Fatal("expected error when HTTP port is occupied")
	}
}

func TestRunAgentRejectsPublicAddress(t *testing.T) {
	cfg := config.Config{
		APIListen: "0.0.0.0:8080",
		APIToken:  "test-api-token",
	}

	if err := agent.Run(context.Background(), cfg); err == nil {
		t.Fatal("expected public listen address rejection")
	}
}
