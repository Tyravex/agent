package panelws

import (
	"context"
	"errors"
	"testing"

	commands "github.com/Tyravex/agent/internal/command"
)

// TestOwnershipGenerationRejectsTakeoverBetweenCheckAndExecute
// reproduces the authorization gap between the transport's
// initial CheckActive and the asynchronous dispatcher call.
func TestOwnershipGenerationRejectsTakeoverBetweenCheckAndExecute(
	t *testing.T,
) {
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

	// This corresponds to the initial check in readLoop.
	if err := oldConnection.CheckActive(ctx); err != nil {
		t.Fatalf("old connection should initially be active: %v", err)
	}

	// Deterministic takeover: the new session becomes active
	// after CheckActive but before Execute.
	newConnection, err := connections.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer newConnection.Close()

	if err := newConnection.Activate(ctx, "new-session"); err != nil {
		t.Fatal(err)
	}

	if got := connections.ActiveGeneration(); got != newConnection.Generation() {
		t.Fatalf("new generation was not activated: got %d", got)
	}

	command := commands.Command{
		ID:        "command-after-ownership-takeover",
		ServiceID: "minecraft-1",
		Operation: commands.ServiceStart,
	}

	// A transport-only CheckActive would already be stale.
	// The generation-bound dispatcher must reject execution.
	err = oldExecutor.Execute(ctx, command)

	if !errors.Is(err, commands.ErrOwnershipGenerationMismatch) {
		t.Fatalf(
			"expected generation mismatch after takeover, got %v",
			err,
		)
	}

	if got := runtime.callCount(); got != 0 {
		t.Fatalf(
			"stale command reached Docker runtime: %d calls",
			got,
		)
	}
}
