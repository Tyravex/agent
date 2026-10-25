package transport

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestCommandIDRegistryRejectsDuplicate(t *testing.T) {
	registry := newCommandIDRegistry()

	if err := registry.reserve("command-1"); err != nil {
		t.Fatalf("first reservation: %v", err)
	}

	err := registry.reserve("command-1")
	if !errors.Is(err, ErrDuplicateCommandID) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestCommandIDRegistryConcurrentReservation(t *testing.T) {
	registry := newCommandIDRegistry()

	const attempts = 64

	var wg sync.WaitGroup
	results := make(chan error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			results <- registry.reserve("shared-command")
		}()
	}

	wg.Wait()
	close(results)

	successes := 0
	duplicates := 0

	for err := range results {
		switch {
		case err == nil:
			successes++

		case errors.Is(err, ErrDuplicateCommandID):
			duplicates++

		default:
			t.Fatalf(
				"unexpected reservation error: %v",
				err,
			)
		}
	}

	if successes != 1 {
		t.Fatalf(
			"expected exactly one success, got %d",
			successes,
		)
	}

	if duplicates != attempts-1 {
		t.Fatalf(
			"expected %d duplicates, got %d",
			attempts-1,
			duplicates,
		)
	}
}

func TestCommandIDRegistryFailsClosedAtCapacity(t *testing.T) {
	registry := newCommandIDRegistry()

	for i := 0; i < maxRememberedCommands; i++ {
		id := fmt.Sprintf("command-%d", i)

		if err := registry.reserve(id); err != nil {
			t.Fatalf("reserve %q: %v", id, err)
		}
	}

	err := registry.reserve("new-command")
	if !errors.Is(err, ErrCommandHistoryFull) {
		t.Fatalf("expected history full, got %v", err)
	}

	// Previously recorded IDs must still be recognized.
	err = registry.reserve("command-0")
	if !errors.Is(err, ErrDuplicateCommandID) {
		t.Fatalf(
			"old command ID was forgotten: %v",
			err,
		)
	}
}

func TestCommandIDRegistrySharedAcrossSessions(t *testing.T) {
	registry := newCommandIDRegistry()

	firstSession := commandExecutionState{
		registry: registry,
	}

	secondSession := commandExecutionState{
		registry: registry,
	}

	if err := firstSession.registry.reserve(
		"reconnect-command",
	); err != nil {
		t.Fatalf("first session: %v", err)
	}

	err := secondSession.registry.reserve(
		"reconnect-command",
	)

	if !errors.Is(err, ErrDuplicateCommandID) {
		t.Fatalf(
			"second session accepted duplicate: %v",
			err,
		)
	}
}
