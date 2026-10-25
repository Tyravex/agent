package transport

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tyravex/agent/internal/commands"
)

type fakeCommandExecutor struct {
	calls []commands.Command
	err   error
}

func (f *fakeCommandExecutor) Execute(
	ctx context.Context,
	command commands.Command,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	f.calls = append(f.calls, command)
	return f.err
}

func TestCommandExecutorUnavailable(t *testing.T) {
	var executor CommandExecutor

	if executor != nil {
		t.Fatal("expected nil executor")
	}

	if !errors.Is(
		ErrCommandExecutorUnavailable,
		ErrCommandExecutorUnavailable,
	) {
		t.Fatal("expected executor unavailable error")
	}
}

func TestLifecycleCommandConvertsToDispatcherCommand(
	t *testing.T,
) {
	raw := json.RawMessage(`{
		"commandId":"command-1",
		"operation":"service.start",
		"serviceId":"service-1",
		"parameters":{}
	}`)

	decoded, err := DecodeServiceLifecycleCommand(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	command, err := decoded.DispatcherCommand()
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	if command.Operation != commands.ServiceStart ||
		command.ID != "command-1" ||
		command.ServiceID != "service-1" {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestFakeExecutorReportsFailure(t *testing.T) {
	executor := &fakeCommandExecutor{
		err: errors.New("private docker failure"),
	}

	err := executor.Execute(
		context.Background(),
		commands.Command{
			ID:        "command-1",
			ServiceID: "service-1",
			Operation: commands.ServiceStart,
		},
	)

	if err == nil {
		t.Fatal("expected execution failure")
	}

	if len(executor.calls) != 1 {
		t.Fatalf("expected one call, got %d", len(executor.calls))
	}
}
