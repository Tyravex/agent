package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	maxCommandIDLength    = 128
	maxCommandErrorLength = 128
	maxCommandMessageLen  = 1024
)

var (
	ErrInvalidCommandResponse = errors.New(
		"invalid command response",
	)
)

// CommandAccepted is sent after a command has been accepted
// for execution, not after the operation has completed.
type CommandAccepted struct {
	CommandID string `json:"commandId"`
}

// CommandResult reports the final outcome of a command.
//
// Result is optional structured metadata. It must never
// contain secrets, credentials or environment variable values.
type CommandResult struct {
	CommandID string         `json:"commandId"`
	Success   bool           `json:"success"`
	ErrorCode string         `json:"errorCode,omitempty"`
	Message   string         `json:"message,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
}

func (r CommandAccepted) Validate() error {
	if err := validateResponseCommandID(r.CommandID); err != nil {
		return err
	}
	return nil
}

func (r CommandResult) Validate() error {
	if err := validateResponseCommandID(r.CommandID); err != nil {
		return err
	}

	if len(r.ErrorCode) > maxCommandErrorLength {
		return fmt.Errorf(
			"%w: error code too long",
			ErrInvalidCommandResponse,
		)
	}

	if len(r.Message) > maxCommandMessageLen {
		return fmt.Errorf(
			"%w: message too long",
			ErrInvalidCommandResponse,
		)
	}

	if r.Success {
		if r.ErrorCode != "" {
			return fmt.Errorf(
				"%w: successful command cannot have an error code",
				ErrInvalidCommandResponse,
			)
		}
	} else {
		if strings.TrimSpace(r.ErrorCode) == "" {
			return fmt.Errorf(
				"%w: failed command requires an error code",
				ErrInvalidCommandResponse,
			)
		}
	}

	if r.Result != nil {
		encoded, err := json.Marshal(r.Result)
		if err != nil {
			return fmt.Errorf(
				"%w: invalid result: %v",
				ErrInvalidCommandResponse,
				err,
			)
		}

		if len(encoded) > maxMessageBytes/2 {
			return fmt.Errorf(
				"%w: result metadata too large",
				ErrInvalidCommandResponse,
			)
		}
	}

	return nil
}

func validateResponseCommandID(value string) error {
	if strings.TrimSpace(value) == "" ||
		len(value) > maxCommandIDLength {
		return fmt.Errorf(
			"%w: invalid command ID",
			ErrInvalidCommandResponse,
		)
	}

	return nil
}

// NewSuccessfulCommandResult creates a successful result
// without exposing internal execution details.
func NewSuccessfulCommandResult(
	commandID string,
) (CommandResult, error) {
	result := CommandResult{
		CommandID: commandID,
		Success:   true,
	}

	return result, result.Validate()
}

// NewFailedCommandResult creates a failed result with a
// public, bounded error message.
//
// Callers must pass sanitized messages, not raw Docker
// stderr or values containing credentials.
func NewFailedCommandResult(
	commandID string,
	errorCode string,
	message string,
) (CommandResult, error) {
	result := CommandResult{
		CommandID: commandID,
		Success:   false,
		ErrorCode: errorCode,
		Message:   message,
	}

	return result, result.Validate()
}
