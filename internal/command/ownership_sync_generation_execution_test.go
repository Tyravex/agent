package commands

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestOwnershipGenerationBoundExecution(t *testing.T) {
	ctx := context.Background()
	syncManager := NewOwnershipSyncManager("node-1")
	connections := NewOwnershipConnectionManager(syncManager)

	activate := func(sessionID string) *OwnershipConnection {
		t.Helper()

		connection, err := connections.Open(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if err := connection.Activate(ctx, sessionID); err != nil {
			t.Fatal(err)
		}

		snapshot, err := json.Marshal(map[string]any{
			"type":        "ownership.snapshot",
			"session_id":  sessionID,
			"node_id":     "node-1",
			"revision":    connection.Generation(),
			"ttl_seconds": 60,
			"assignments": []any{
				map[string]any{
					"service_id": "minecraft-1",
					"node_id":    "node-1",
					"enabled":    true,
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		if err := connection.ApplySnapshot(ctx, snapshot); err != nil {
			t.Fatal(err)
		}

		if got := connections.Status().State; got != OwnershipReady {
			t.Fatalf("expected READY, got %s", got)
		}

		return connection
	}

	oldConnection := activate("old-session")
	defer oldConnection.Close()

	// The original generation can execute while active.
	executions := 0
	run := func() error {
		executions++
		return nil
	}

	if err := syncManager.WithAuthorizedServiceForGeneration(
		ctx, oldConnection.Generation(), "node-1", "minecraft-1", run,
	); err != nil {
		t.Fatalf("active generation rejected: %v", err)
	}

	newConnection := activate("new-session")
	defer newConnection.Close()

	// Even though the NEW generation is READY, the OLD
	// generation must never borrow its authorization.
	err := syncManager.WithAuthorizedServiceForGeneration(
		ctx, oldConnection.Generation(), "node-1", "minecraft-1", run,
	)

	if !errors.Is(err, ErrOwnershipGenerationMismatch) {
		t.Fatalf("expected generation mismatch, got %v", err)
	}

	if executions != 1 {
		t.Fatalf("stale generation executed: count=%d", executions)
	}

	// The current generation remains authorized.
	if err := syncManager.WithAuthorizedServiceForGeneration(
		ctx, newConnection.Generation(), "node-1", "minecraft-1", run,
	); err != nil {
		t.Fatalf("current generation rejected: %v", err)
	}

	if executions != 2 {
		t.Fatalf("expected two authorized executions, got %d", executions)
	}
}
