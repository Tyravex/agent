package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/config"
)

type ownershipRuntimeStub struct{}

func (*ownershipRuntimeStub) Start(context.Context, string) error {
	return nil
}

func (*ownershipRuntimeStub) Stop(context.Context, string) error {
	return nil
}

func (*ownershipRuntimeStub) Restart(context.Context, string) error {
	return nil
}

func TestPrepareOwnershipExecution(t *testing.T) {
	valid := config.Config{
		PanelURL:      "https://panel.example",
		NodeID:        "node-1",
		NodeToken:     "test-token",
		DockerEnabled: true,
	}

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		runtime bool
		wantErr bool
	}{
		{
			name:    "valid configuration",
			runtime: true,
		},
		{
			name:    "docker disabled",
			mutate:  func(c *config.Config) { c.DockerEnabled = false },
			runtime: true,
			wantErr: true,
		},
		{
			name:    "insecure panel",
			mutate:  func(c *config.Config) { c.PanelURL = "http://localhost:8080" },
			runtime: true,
			wantErr: true,
		},
		{
			name:    "missing node ID",
			mutate:  func(c *config.Config) { c.NodeID = "" },
			runtime: true,
			wantErr: true,
		},
		{
			name:    "missing token",
			mutate:  func(c *config.Config) { c.NodeToken = "" },
			runtime: true,
			wantErr: true,
		},
		{
			name:    "explicit docker host",
			mutate:  func(c *config.Config) { c.DockerHost = "tcp://127.0.0.1:2375" },
			runtime: true,
			wantErr: true,
		},
		{
			name:    "missing runtime",
			runtime: false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid

			if tt.mutate != nil {
				tt.mutate(&cfg)
			}

			var runtime *ownershipRuntimeStub
			if tt.runtime {
				runtime = &ownershipRuntimeStub{}
			}

			var executionRuntime interface {
				Start(context.Context, string) error
				Stop(context.Context, string) error
				Restart(context.Context, string) error
			}

			if runtime != nil {
				executionRuntime = runtime
			}

			execution, err := prepareOwnershipExecution(
				cfg,
				executionRuntime,
			)

			if tt.wantErr {
				if !errors.Is(err, ErrRunnerOwnershipExecutionUnavailable) {
					t.Fatalf("expected unavailable error, got %v", err)
				}
				if execution != nil {
					t.Fatal("expected nil execution")
				}
				return
			}

			if err != nil {
				t.Fatal(err)
			}
			if execution == nil {
				t.Fatal("expected ownership execution")
			}
		})
	}
}
