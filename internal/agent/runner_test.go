package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tyravex/agent/internal/config"
)

func TestRunnerInitialization(t *testing.T) {
	cfg := config.Config{
		PanelURL:  "http://127.0.0.1:3000",
		NodeID:    "test-node",
		NodeToken: "test-token",
	}

	before := time.Now()
	runner := NewRunner(cfg)
	after := time.Now()

	if runner == nil {
		t.Fatal("expected Runner")
	}

	if runner.transport == nil {
		t.Fatal("expected WebSocket transport")
	}

	if runner.config.NodeID != cfg.NodeID {
		t.Fatalf("unexpected node ID: %q", runner.config.NodeID)
	}

	if runner.startedAt.Before(before) || runner.startedAt.After(after) {
		t.Fatal("invalid Runner start time")
	}
}

func TestRunnerDockerDisabled(t *testing.T) {
	runner := NewRunner(config.Config{
		DockerEnabled: false,
	})

	status := runner.dockerStatus(context.Background())

	if status.Available {
		t.Fatal("Docker should be unavailable when disabled")
	}

	if status.Version != nil {
		t.Fatal("Docker version should be nil when disabled")
	}
}

func TestWaitForReconnectCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()

	if waitForReconnect(ctx, 5*time.Second) {
		t.Fatal("expected reconnect wait to stop")
	}

	if time.Since(start) > time.Second {
		t.Fatal("cancelled reconnect took too long")
	}
}

func TestWaitForReconnectDelay(t *testing.T) {
	ctx := context.Background()
	delay := 20 * time.Millisecond

	start := time.Now()

	if !waitForReconnect(ctx, delay) {
		t.Fatal("expected reconnect wait to complete")
	}

	if time.Since(start) < delay {
		t.Fatal("reconnect wait finished too early")
	}
}

func TestRunnerStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := NewRunner(config.Config{
		PanelURL:  "http://127.0.0.1:3000",
		NodeID:    "test-node",
		NodeToken: "test-token",
	})

	done := make(chan struct{})

	go func() {
		defer close(done)
		runner.Run(ctx)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Runner did not stop after cancellation")
	}
}

func TestReconnectContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Millisecond,
	)
	defer cancel()

	if waitForReconnect(ctx, 5*time.Second) {
		t.Fatal("expected reconnect to stop at deadline")
	}

	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", ctx.Err())
	}
}
