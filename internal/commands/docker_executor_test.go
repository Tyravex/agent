package commands

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeDockerServiceRuntime struct {
	calls []string
	err   error
}

func (f *fakeDockerServiceRuntime) Start(
	ctx context.Context,
	serviceID string,
) error {
	f.calls = append(f.calls, "start:"+serviceID)
	return f.err
}

func (f *fakeDockerServiceRuntime) Stop(
	ctx context.Context,
	serviceID string,
) error {
	f.calls = append(f.calls, "stop:"+serviceID)
	return f.err
}

func (f *fakeDockerServiceRuntime) Restart(
	ctx context.Context,
	serviceID string,
) error {
	f.calls = append(f.calls, "restart:"+serviceID)
	return f.err
}

func TestDockerExecutorOperations(t *testing.T) {
	tests := []struct {
		name      string
		operation Operation
		expected  string
	}{
		{
			name:      "start",
			operation: ServiceStart,
			expected:  "start:service-1",
		},
		{
			name:      "stop",
			operation: ServiceStop,
			expected:  "stop:service-1",
		},
		{
			name:      "restart",
			operation: ServiceRestart,
			expected:  "restart:service-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &fakeDockerServiceRuntime{}
			executor := NewDockerExecutor(runtime)
			dispatcher := NewDispatcher(executor, nil)

			err := dispatcher.Execute(
				context.Background(),
				Command{
					ID:        "command-1",
					ServiceID: "service-1",
					Operation: tt.operation,
				},
			)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}

			if len(runtime.calls) != 1 ||
				runtime.calls[0] != tt.expected {
				t.Fatalf(
					"unexpected Docker calls: %v",
					runtime.calls,
				)
			}
		})
	}
}

func TestDockerExecutorPropagatesError(t *testing.T) {
	expected := errors.New("Docker Engine offline")
	runtime := &fakeDockerServiceRuntime{
		err: expected,
	}

	executor := NewDockerExecutor(runtime)

	err := executor.Start(
		context.Background(),
		"service-1",
	)

	if !errors.Is(err, expected) {
		t.Fatalf(
			"expected wrapped Docker error, got %v",
			err,
		)
	}

	if !strings.Contains(err.Error(), "service-1") {
		t.Fatalf(
			"expected service ID in error: %v",
			err,
		)
	}
}

func TestDockerExecutorRejectsNilRuntime(t *testing.T) {
	executor := NewDockerExecutor(nil)

	err := executor.Start(
		context.Background(),
		"service-1",
	)

	if !errors.Is(err, ErrDockerRuntimeUnavailable) {
		t.Fatalf(
			"expected unavailable runtime, got %v",
			err,
		)
	}
}

func TestDockerExecutorRejectsMissingServiceID(t *testing.T) {
	runtime := &fakeDockerServiceRuntime{}
	executor := NewDockerExecutor(runtime)

	err := executor.Start(
		context.Background(),
		" ",
	)

	if !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf(
			"expected invalid command, got %v",
			err,
		)
	}

	if len(runtime.calls) != 0 {
		t.Fatal("invalid service ID reached Docker")
	}
}

func TestDockerExecutorHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	runtime := &fakeDockerServiceRuntime{}
	executor := NewDockerExecutor(runtime)

	err := executor.Stop(ctx, "service-1")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"expected cancellation, got %v",
			err,
		)
	}

	if len(runtime.calls) != 0 {
		t.Fatal("cancelled command reached Docker")
	}
}

func TestDockerExecutorCanUseDispatcher(t *testing.T) {
	runtime := &fakeDockerServiceRuntime{}
	executor := NewDockerExecutor(runtime)
	dispatcher := NewDispatcher(executor, nil)

	err := dispatcher.Execute(
		context.Background(),
		Command{
			ID:        "command-42",
			ServiceID: "service-42",
			Operation: ServiceRestart,
		},
	)

	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(runtime.calls) != 1 ||
		runtime.calls[0] != "restart:service-42" {
		t.Fatalf(
			"unexpected calls: %v",
			runtime.calls,
		)
	}
}
