package commands

import (
	"context"
	"errors"
	"testing"
)

type fakeExecutor struct {
	calls []string
	err   error
}

func (f *fakeExecutor) Start(
	ctx context.Context,
	serviceID string,
) error {
	f.calls = append(f.calls, "start:"+serviceID)
	return f.err
}

func (f *fakeExecutor) Stop(
	ctx context.Context,
	serviceID string,
) error {
	f.calls = append(f.calls, "stop:"+serviceID)
	return f.err
}

func (f *fakeExecutor) Restart(
	ctx context.Context,
	serviceID string,
) error {
	f.calls = append(f.calls, "restart:"+serviceID)
	return f.err
}

type fakeInstaller struct {
	calls []Command
	err   error
}

func (f *fakeInstaller) Install(
	ctx context.Context,
	command Command,
) error {
	f.calls = append(f.calls, command)
	return f.err
}

func TestDispatcherServiceOperations(t *testing.T) {
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
			executor := &fakeExecutor{}
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

			if len(executor.calls) != 1 ||
				executor.calls[0] != tt.expected {
				t.Fatalf(
					"unexpected calls: %v",
					executor.calls,
				)
			}
		})
	}
}

func TestDispatcherInstallation(t *testing.T) {
	installer := &fakeInstaller{}
	dispatcher := NewDispatcher(nil, installer)

	command := Command{
		ID:        "command-2",
		ServiceID: "service-2",
		Operation: ServiceInstall,
	}

	if err := dispatcher.Execute(
		context.Background(),
		command,
	); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(installer.calls) != 1 {
		t.Fatalf(
			"expected one installation call, got %d",
			len(installer.calls),
		)
	}

	if installer.calls[0] != command {
		t.Fatal("installation command was changed")
	}
}

func TestDispatcherRejectsInvalidCommands(t *testing.T) {
	tests := []struct {
		name    string
		command Command
		want    error
	}{
		{
			name: "missing command ID",
			command: Command{
				ServiceID: "service-1",
				Operation: ServiceStart,
			},
			want: ErrInvalidCommand,
		},
		{
			name: "missing service ID",
			command: Command{
				ID:        "command-1",
				Operation: ServiceStart,
			},
			want: ErrInvalidCommand,
		},
		{
			name: "unsupported operation",
			command: Command{
				ID:        "command-1",
				ServiceID: "service-1",
				Operation: "service.delete",
			},
			want: ErrUnsupportedOperation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &fakeExecutor{}
			dispatcher := NewDispatcher(executor, nil)

			err := dispatcher.Execute(
				context.Background(),
				tt.command,
			)

			if !errors.Is(err, tt.want) {
				t.Fatalf(
					"expected %v, got %v",
					tt.want,
					err,
				)
			}

			if len(executor.calls) != 0 {
				t.Fatal("invalid command reached executor")
			}
		})
	}
}

func TestDispatcherRejectsUnavailableExecutor(t *testing.T) {
	dispatcher := NewDispatcher(nil, nil)

	for _, operation := range []Operation{
		ServiceInstall,
		ServiceStart,
		ServiceStop,
		ServiceRestart,
	} {
		err := dispatcher.Execute(
			context.Background(),
			Command{
				ID:        "command-1",
				ServiceID: "service-1",
				Operation: operation,
			},
		)

		if !errors.Is(err, ErrExecutorUnavailable) {
			t.Fatalf(
				"%s: expected unavailable executor, got %v",
				operation,
				err,
			)
		}
	}
}

func TestDispatcherPropagatesExecutorError(t *testing.T) {
	expected := errors.New("Docker unavailable")
	executor := &fakeExecutor{err: expected}
	dispatcher := NewDispatcher(executor, nil)

	err := dispatcher.Execute(
		context.Background(),
		Command{
			ID:        "command-1",
			ServiceID: "service-1",
			Operation: ServiceStart,
		},
	)

	if !errors.Is(err, expected) {
		t.Fatalf(
			"expected executor error, got %v",
			err,
		)
	}
}

func TestDispatcherHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	executor := &fakeExecutor{}
	dispatcher := NewDispatcher(executor, nil)

	err := dispatcher.Execute(
		ctx,
		Command{
			ID:        "command-1",
			ServiceID: "service-1",
			Operation: ServiceStart,
		},
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"expected cancellation, got %v",
			err,
		)
	}

	if len(executor.calls) != 0 {
		t.Fatal("cancelled command reached executor")
	}
}
