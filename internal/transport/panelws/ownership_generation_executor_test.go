package panelws

import (
	"context"
	"errors"
	"testing"

	commands "github.com/Tyravex/agent/internal/command"
)

func TestOwnershipGenerationExecutorRejectsSupersededSession(t *testing.T) {
	ctx := context.Background()

	syncManager := commands.NewOwnershipSyncManager("node-1")
	connections := commands.NewOwnershipConnectionManager(syncManager)
	runtime := &recordingOwnershipRuntime{}

	binder := &ownershipExecutorBinder{
		nodeID:      "node-1",
		syncManager: syncManager,
		runtime:     runtime,
	}

	oldConnection, err := connections.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer oldConnection.Close()

	if err := oldConnection.Activate(ctx, "old-session"); err != nil {
		t.Fatal(err)
	}

	oldExecutor := binder.BindOwnershipGeneration(
		oldConnection.Generation(),
	)

	newConnection, err := connections.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer newConnection.Close()

	if err := newConnection.Activate(ctx, "new-session"); err != nil {
		t.Fatal(err)
	}

	newExecutor := binder.BindOwnershipGeneration(
		newConnection.Generation(),
	)

	command := commands.Command{
		ID:        "stale-generation-command",
		ServiceID: "minecraft-1",
		Operation: commands.ServiceStart,
	}

	// A superseded executor must be rejected before the
	// service runtime can be reached.
	err = oldExecutor.Execute(ctx, command)

	if !errors.Is(err, commands.ErrOwnershipGenerationMismatch) {
		t.Fatalf(
			"expected generation mismatch, got %v",
			err,
		)
	}

	if got := runtime.callCount(); got != 0 {
		t.Fatalf(
			"superseded executor reached runtime: %d calls",
			got,
		)
	}

	// The binder itself must never execute commands.
	err = binder.Execute(ctx, command)

	if !errors.Is(err, commands.ErrOwnershipGenerationMismatch) {
		t.Fatalf(
			"unbound binder must fail closed, got %v",
			err,
		)
	}

	if got := runtime.callCount(); got != 0 {
		t.Fatalf(
			"unbound binder reached runtime: %d calls",
			got,
		)
	}

	// Keep a reference to the new executor to ensure
	// both generations were constructed independently.
	if newExecutor == nil {
		t.Fatal("new generation executor is nil")
	}
}
