package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tyravex/agent/internal/commands"
)

var ErrInvalidServiceLifecycleCommand = errors.New(
	"invalid service lifecycle command",
)

// ServiceLifecycleCommand represents a validated command.execute
// payload for start, stop or restart.
//
// Installation commands are handled by the separate installation
// decoder and must not be accepted here.
type ServiceLifecycleCommand struct {
	CommandID  string             `json:"commandId"`
	Operation  commands.Operation `json:"operation"`
	ServiceID  string             `json:"serviceId"`
	Parameters json.RawMessage    `json:"parameters"`
}

// DecodeServiceLifecycleCommand validates the lifecycle command
// received from the Panel.
//
// Lifecycle operations currently require an empty parameters object.
// Unknown fields, malformed JSON and duplicate JSON keys are rejected.
func DecodeServiceLifecycleCommand(
	data json.RawMessage,
) (ServiceLifecycleCommand, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return ServiceLifecycleCommand{}, fmt.Errorf(
			"%w: %v",
			ErrInvalidServiceLifecycleCommand,
			err,
		)
	}

	var command ServiceLifecycleCommand
	if err := decodeStrict(data, &command); err != nil {
		return ServiceLifecycleCommand{}, fmt.Errorf(
			"%w: %v",
			ErrInvalidServiceLifecycleCommand,
			err,
		)
	}

	if err := command.Validate(); err != nil {
		return ServiceLifecycleCommand{}, err
	}

	return command, nil
}

// Validate checks the command identity, operation and parameters.
func (c ServiceLifecycleCommand) Validate() error {
	if strings.TrimSpace(c.CommandID) == "" ||
		len(c.CommandID) > 128 {
		return fmt.Errorf(
			"%w: invalid command ID",
			ErrInvalidServiceLifecycleCommand,
		)
	}

	if strings.TrimSpace(c.ServiceID) == "" {
		return fmt.Errorf(
			"%w: missing service ID",
			ErrInvalidServiceLifecycleCommand,
		)
	}

	switch c.Operation {
	case commands.ServiceStart,
		commands.ServiceStop,
		commands.ServiceRestart:
	default:
		return fmt.Errorf(
			"%w: unsupported operation %q",
			ErrInvalidServiceLifecycleCommand,
			c.Operation,
		)
	}

	if len(c.Parameters) == 0 {
		return fmt.Errorf(
			"%w: missing parameters",
			ErrInvalidServiceLifecycleCommand,
		)
	}

	var parameters map[string]json.RawMessage
	if err := json.Unmarshal(
		c.Parameters,
		&parameters,
	); err != nil {
		return fmt.Errorf(
			"%w: invalid parameters: %v",
			ErrInvalidServiceLifecycleCommand,
			err,
		)
	}

	if parameters == nil || len(parameters) != 0 {
		return fmt.Errorf(
			"%w: lifecycle parameters must be an empty object",
			ErrInvalidServiceLifecycleCommand,
		)
	}

	return nil
}

// DispatcherCommand converts the validated transport command
// into the internal commands package representation.
func (c ServiceLifecycleCommand) DispatcherCommand() (
	commands.Command,
	error,
) {
	if err := c.Validate(); err != nil {
		return commands.Command{}, err
	}

	return commands.Command{
		ID:        c.CommandID,
		ServiceID: c.ServiceID,
		Operation: c.Operation,
	}, nil
}
