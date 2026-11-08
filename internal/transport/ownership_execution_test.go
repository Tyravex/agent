package transport

import (
	"context"
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/commands"
)

type ownershipExecutionTestRuntime struct{}

func (*ownershipExecutionTestRuntime) Start(context.Context, string) error {
	return nil
}

func (*ownershipExecutionTestRuntime) Stop(context.Context, string) error {
	return nil
}

func (*ownershipExecutionTestRuntime) Restart(context.Context, string) error {
	return nil
}

func TestNewOwnershipExecutionRejectsInvalidDependencies(t *testing.T) {
	syncManager := commands.NewOwnershipSyncManager("node-1")
	runtime := &ownershipExecutionTestRuntime{}

	tests := []struct {
		name    string
		nodeID  string
		manager *commands.OwnershipSyncManager
		runtime commands.DockerServiceRuntime
	}{
		{
			name:    "missing node ID",
			manager: syncManager,
			runtime: runtime,
		},
		{
			name:    "missing sync manager",
			nodeID:  "node-1",
			runtime: runtime,
		},
		{
			name:    "missing runtime",
			nodeID:  "node-1",
			manager: syncManager,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execution, err := NewOwnershipExecution(
				tt.nodeID,
				tt.manager,
				tt.runtime,
			)
			if !errors.Is(err, ErrOwnershipExecutionUnavailable) {
				t.Fatalf("expected unavailable error, got %v", err)
			}
			if execution != nil {
				t.Fatal("expected nil execution")
			}
		})
	}
}

func TestNewOwnershipExecutionSharesConnectionManager(t *testing.T) {
	syncManager := commands.NewOwnershipSyncManager("node-1")

	execution, err := NewOwnershipExecution(
		"node-1",
		syncManager,
		&ownershipExecutionTestRuntime{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if execution.connections == nil || execution.dispatcher == nil {
		t.Fatal("expected connection manager and dispatcher")
	}

	first, err := execution.connections.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	second, err := execution.connections.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if second.Generation() <= first.Generation() {
		t.Fatal("connection generation did not increase")
	}
}

func TestConnectWithOwnershipExecutionRejectsNil(t *testing.T) {
	var client *Client

	_, err := client.ConnectWithOwnershipExecution(
		context.Background(),
		nil,
	)

	if !errors.Is(err, ErrOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestOwnershipExecutionDeniesWithoutReadySnapshot(t *testing.T) {
	syncManager := commands.NewOwnershipSyncManager("node-1")

	execution, err := NewOwnershipExecution(
		"node-1",
		syncManager,
		&ownershipExecutionTestRuntime{},
	)
	if err != nil {
		t.Fatal(err)
	}

	command := commands.Command{
		ID:        "command-1",
		Operation: commands.ServiceStart,
		ServiceID: "service-1",
	}

	err = execution.dispatcher.Execute(context.Background(), command)
	if !errors.Is(err, commands.ErrOwnershipNotReady) {
		t.Fatalf("expected ownership not ready, got %v", err)
	}
}

func TestConnectWithOwnershipExecutionRequiresTLS(t *testing.T) {
	syncManager := commands.NewOwnershipSyncManager("node-1")

	execution, err := NewOwnershipExecution(
		"node-1",
		syncManager,
		&ownershipExecutionTestRuntime{},
	)
	if err != nil {
		t.Fatal(err)
	}

	client := &Client{}
	client.config.PanelURL = "http://localhost:8080"

	session, err := client.ConnectWithOwnershipExecution(
		context.Background(),
		execution,
	)

	if !errors.Is(err, ErrOwnershipRequiresTLS) {
		t.Fatalf("expected TLS requirement, got %v", err)
	}
	if session != nil {
		t.Fatal("expected nil session")
	}
}

func TestNewOwnershipExecutionRejectsNodeMismatch(t *testing.T) {
	syncManager := commands.NewOwnershipSyncManager("node-1")

	execution, err := NewOwnershipExecution(
		"node-2",
		syncManager,
		&ownershipExecutionTestRuntime{},
	)

	if !errors.Is(err, ErrOwnershipExecutionUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}

	if execution != nil {
		t.Fatal("expected nil execution")
	}
}
