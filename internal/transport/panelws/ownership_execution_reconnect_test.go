package panelws

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	commands "github.com/Tyravex/agent/internal/command"
)

func TestOwnershipExecutionOldSessionCannotExecuteAfterReconnect(t *testing.T) {
	ctx := context.Background()

	syncManager := commands.NewOwnershipSyncManager("node-1")
	runtime := &recordingOwnershipRuntime{}

	execution, err := NewOwnershipExecution(
		"node-1",
		syncManager,
		runtime,
	)
	if err != nil {
		t.Fatal(err)
	}

	oldConnection, err := execution.connections.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer oldConnection.Close()

	if err := oldConnection.Activate(ctx, "old-session"); err != nil {
		t.Fatal(err)
	}

	snapshot := ownershipIntegrationSnapshot("old-session", 1)

	// Encode the snapshot using the same JSON format as the Panel.
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	if err := oldConnection.ApplySnapshot(ctx, data); err != nil {
		t.Fatal(err)
	}

	if got := execution.connections.Status().State; got != commands.OwnershipReady {
		t.Fatalf("old session should be READY, got %s", got)
	}

	newConnection, err := execution.connections.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer newConnection.Close()

	if err := newConnection.Activate(ctx, "new-session"); err != nil {
		t.Fatal(err)
	}

	// The new session must invalidate the old authorization
	// even while the old transport connection still exists.
	if got := execution.connections.Status().State; got != commands.OwnershipNotReady {
		t.Fatalf("new session should start NOT_READY, got %s", got)
	}

	command := commands.Command{
		ID:        "old-session-command",
		ServiceID: "minecraft-1",
		Operation: commands.ServiceStart,
	}

	oldExecutor := execution.binder.BindOwnershipGeneration(
		oldConnection.Generation(),
	)
	err = oldExecutor.Execute(ctx, command)
	if !errors.Is(err, commands.ErrOwnershipGenerationMismatch) {
		t.Fatalf("expected ownership generation mismatch, got %v", err)
	}

	if got := runtime.callCount(); got != 0 {
		t.Fatalf("old authorization executed Docker operation: %d calls", got)
	}

	// A delayed disconnect from the old session must not
	// revoke the newer connection.
	oldConnection.Close()

	if got := execution.connections.ActiveGeneration(); got != newConnection.Generation() {
		t.Fatalf("old disconnect revoked new generation: %d", got)
	}
}
