package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/Tyravex/agent/internal/commands"
)

const maxRememberedCommands = 4096

var ErrCommandExecutorUnavailable = errors.New(
	"command executor unavailable",
)

// CommandExecutor executes validated lifecycle commands.
type CommandExecutor interface {
	Execute(ctx context.Context, command commands.Command) error
}

type commandExecutionState struct {
	mu       sync.Mutex
	executor CommandExecutor
	seen     map[string]struct{}

	// Only one lifecycle command may execute at a time.
	active bool
	done   chan struct{}
}

// SetCommandExecutor is retained for compatibility.
//
// Prefer Client.ConnectWithExecutor: it installs the executor
// before the WebSocket read loop starts.
func (s *Session) SetCommandExecutor(executor CommandExecutor) error {
	if executor == nil {
		return ErrCommandExecutorUnavailable
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errSessionClosed
	}

	s.commandState.mu.Lock()
	defer s.commandState.mu.Unlock()

	if len(s.commandState.seen) != 0 {
		return errors.New("command execution already started")
	}

	s.commandState.executor = executor
	return nil
}

func (s *Session) handleCommand(
	ctx context.Context,
	payload json.RawMessage,
) error {
	var raw struct {
		Operation  string          `json:"operation"`
		Parameters json.RawMessage `json:"parameters"`
	}

	if err := json.Unmarshal(payload, &raw); err != nil {
		return fmt.Errorf("invalid command operation: %w", err)
	}

	// Installation-shaped commands must always go through
	// the installation decoder, including invalid operations.
	isInstallation := raw.Operation == serviceInstallOperation

	if !isInstallation && len(raw.Parameters) > 0 {
		var parameters map[string]json.RawMessage

		if err := json.Unmarshal(
			raw.Parameters,
			&parameters,
		); err == nil && parameters != nil {
			_, hasFormatVersion := parameters["formatVersion"]
			_, hasEgg := parameters["egg"]
			_, hasRuntime := parameters["runtime"]
			_, hasResources := parameters["resources"]

			isInstallation = hasFormatVersion ||
				hasEgg ||
				hasRuntime ||
				hasResources
		}
	}

	if isInstallation {
		command, err := DecodeServiceInstallCommand(payload)
		if err != nil {
			return fmt.Errorf(
				"invalid service installation command: %w",
				err,
			)
		}

		if command.Install.NodeID != s.welcome.NodeID {
			return errors.New(
				"service installation command targets a different node",
			)
		}

		// Installation remains unsupported.
		// Never acknowledge or execute it.
		return errors.New(
			"service installation commands are not supported yet",
		)
	}

	command, err := DecodeServiceLifecycleCommand(payload)
	if err != nil {
		return fmt.Errorf("invalid lifecycle command: %w", err)
	}

	dispatchCommand, err := command.DispatcherCommand()
	if err != nil {
		return err
	}

	s.commandState.mu.Lock()

	if _, exists := s.commandState.seen[command.CommandID]; exists {
		s.commandState.mu.Unlock()
		return errors.New("duplicate command ID")
	}

	if len(s.commandState.seen) >= maxRememberedCommands {
		s.commandState.mu.Unlock()
		return errors.New("command history limit reached")
	}

	executor := s.commandState.executor
	if executor == nil {
		s.commandState.mu.Unlock()
		return ErrCommandExecutorUnavailable
	}

	// Remember rejected-busy commands as well, so a repeated
	// command ID cannot acquire a different meaning in this session.
	s.commandState.seen[command.CommandID] = struct{}{}

	busy := s.commandState.active

	var executionDone chan struct{}
	if !busy {
		executionDone = make(chan struct{})
		s.commandState.active = true
		s.commandState.done = executionDone
	}

	s.commandState.mu.Unlock()

	accepted := CommandAccepted{
		CommandID: command.CommandID,
	}
	if err := accepted.Validate(); err != nil {
		if !busy {
			s.finishCommand(executionDone)
		}
		return err
	}

	if err := s.sendEnvelope(
		ctx,
		"command.accepted",
		accepted,
	); err != nil {
		if !busy {
			s.finishCommand(executionDone)
		}
		return fmt.Errorf("send command.accepted: %w", err)
	}

	if busy {
		result, err := NewFailedCommandResult(
			command.CommandID,
			"COMMAND_BUSY",
			"Another service operation is in progress",
		)
		if err != nil {
			return err
		}

		if err := s.sendEnvelope(
			ctx,
			"command.result",
			result,
		); err != nil {
			return fmt.Errorf("send busy command.result: %w", err)
		}

		return nil
	}

	go s.executeCommand(
		ctx,
		executor,
		dispatchCommand,
		executionDone,
	)

	return nil
}

func (s *Session) executeCommand(
	ctx context.Context,
	executor CommandExecutor,
	command commands.Command,
	done chan struct{},
) {
	defer s.finishCommand(done)

	err := executor.Execute(ctx, command)

	var result CommandResult

	if err != nil {
		result, _ = NewFailedCommandResult(
			command.ID,
			"EXECUTION_FAILED",
			"Service operation failed",
		)
	} else {
		result, _ = NewSuccessfulCommandResult(command.ID)
	}

	// A disconnected session must not attempt to publish
	// a final result over the old WebSocket.
	if ctx.Err() != nil {
		return
	}

	if err := s.sendEnvelope(
		ctx,
		"command.result",
		result,
	); err != nil {
		s.shutdown(fmt.Errorf(
			"send command.result: %w",
			err,
		))
	}
}

func (s *Session) finishCommand(done chan struct{}) {
	s.commandState.mu.Lock()
	defer s.commandState.mu.Unlock()

	if s.commandState.done != done {
		return
	}

	s.commandState.active = false
	s.commandState.done = nil
	close(done)
}

// WaitCommands waits for the currently active command to finish.
//
// It does not cancel the command. Close() or session context
// cancellation must be used separately.
//
// The caller can supply a deadline to avoid waiting forever
// for an executor that ignores context cancellation.
func (s *Session) WaitCommands(ctx context.Context) error {
	s.commandState.mu.Lock()
	done := s.commandState.done
	s.commandState.mu.Unlock()

	if done == nil {
		return nil
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
