package panelws

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	commands "github.com/Tyravex/agent/internal/command"
)

func TestOwnershipAuthorizationDuringSessionTakeover(t *testing.T) {
	newEnvironment := func(t *testing.T) (
		*commands.OwnershipSyncManager,
		*commands.OwnershipConnectionManager,
		*commands.OwnershipConnection,
	) {
		t.Helper()

		ctx := context.Background()
		syncManager := commands.NewOwnershipSyncManager("node-1")
		manager := commands.NewOwnershipConnectionManager(syncManager)

		oldConnection, err := manager.Open(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if err := oldConnection.Activate(ctx, "old-session"); err != nil {
			t.Fatal(err)
		}

		snapshot, err := json.Marshal(
			ownershipIntegrationSnapshot("old-session", 1),
		)
		if err != nil {
			t.Fatal(err)
		}

		if err := oldConnection.ApplySnapshot(ctx, snapshot); err != nil {
			t.Fatal(err)
		}

		if got := manager.Status().State; got != commands.OwnershipReady {
			t.Fatalf("expected READY, got %s", got)
		}

		return syncManager, manager, oldConnection
	}

	takeover := func(
		t *testing.T,
		manager *commands.OwnershipConnectionManager,
	) *commands.OwnershipConnection {
		t.Helper()

		ctx := context.Background()

		next, err := manager.Open(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if err := next.Activate(ctx, "new-session"); err != nil {
			t.Fatal(err)
		}

		if got := manager.Status().State; got != commands.OwnershipNotReady {
			t.Fatalf("expected NOT_READY after takeover, got %s", got)
		}

		return next
	}

	t.Run("authorized operation may finish after takeover", func(t *testing.T) {
		syncManager, manager, oldConnection := newEnvironment(t)
		defer oldConnection.Close()

		authorized := make(chan struct{})
		release := make(chan struct{})
		finished := make(chan error, 1)

		go func() {
			err := syncManager.WithAuthorizedService(
				context.Background(),
				"node-1",
				"minecraft-1",
				func() error {
					close(authorized)
					<-release
					return nil
				},
			)
			finished <- err
		}()

		// The callback starts only after authorization succeeds.
		<-authorized

		next := takeover(t, manager)
		defer next.Close()

		// Releasing the callback must not require the old
		// ownership generation to remain active.
		close(release)

		if err := <-finished; err != nil {
			t.Fatalf("authorized operation failed: %v", err)
		}
	})

	t.Run("operation starting after takeover is denied", func(t *testing.T) {
		syncManager, manager, oldConnection := newEnvironment(t)
		defer oldConnection.Close()

		next := takeover(t, manager)
		defer next.Close()

		executed := false

		err := syncManager.WithAuthorizedService(
			context.Background(),
			"node-1",
			"minecraft-1",
			func() error {
				executed = true
				return nil
			},
		)

		if !errors.Is(err, commands.ErrOwnershipNotReady) {
			t.Fatalf("expected ErrOwnershipNotReady, got %v", err)
		}

		if executed {
			t.Fatal("operation executed without current authorization")
		}
	})
}
