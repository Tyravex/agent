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
}

// SetCommandExecutor configures lifecycle execution for a session.
//
// It must be called before commands are received. The executor
// cannot be replaced after command processing has started.
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

	// Installation-shaped commands must always be validated by
	// the installation decoder, even when the operation is invalid.
	//
	// This preserves strict rejection of malformed installation
	// requests and prevents them from being interpreted as
	// lifecycle commands.
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

		// Installation execution is not implemented.
		// Never acknowledge an unsupported installation.
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

	s.commandState.seen[command.CommandID] = struct{}{}
	executor := s.commandState.executor

	s.commandState.mu.Unlock()

	if executor == nil {
		return ErrCommandExecutorUnavailable
	}

	accepted := CommandAccepted{
		CommandID: command.CommandID,
	}

	if err := accepted.Validate(); err != nil {
		return err
	}

	if err := s.sendEnvelope(
		ctx,
		"command.accepted",
		accepted,
	); err != nil {
		return fmt.Errorf("send command.accepted: %w", err)
	}

	go s.executeCommand(ctx, executor, dispatchCommand)

	return nil
}

func (s *Session) executeCommand(
	ctx context.Context,
	executor CommandExecutor,
	command commands.Command,
) {
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
