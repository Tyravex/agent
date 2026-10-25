package transport

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCommandAcceptedValidation(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid", "command-123", false},
		{"empty", "", true},
		{"whitespace", "   ", true},
		{"too long", strings.Repeat("a", 129), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (CommandAccepted{
				CommandID: tt.id,
			}).Validate()

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"Validate() error = %v, wantErr %v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}

func TestSuccessfulCommandResult(t *testing.T) {
	result, err := NewSuccessfulCommandResult(
		"command-123",
	)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Success ||
		result.CommandID != "command-123" ||
		result.ErrorCode != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestFailedCommandResult(t *testing.T) {
	result, err := NewFailedCommandResult(
		"command-123",
		"DOCKER_UNAVAILABLE",
		"Docker Engine is unavailable",
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.Success {
		t.Fatal("expected failed result")
	}

	if result.ErrorCode != "DOCKER_UNAVAILABLE" {
		t.Fatalf(
			"unexpected error code: %q",
			result.ErrorCode,
		)
	}
}

func TestCommandResultRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name   string
		result CommandResult
	}{
		{
			name: "missing ID",
			result: CommandResult{
				Success: true,
			},
		},
		{
			name: "success with error code",
			result: CommandResult{
				CommandID: "command-1",
				Success:   true,
				ErrorCode: "FAILED",
			},
		},
		{
			name: "failure without error code",
			result: CommandResult{
				CommandID: "command-1",
				Success:   false,
			},
		},
		{
			name: "error code too long",
			result: CommandResult{
				CommandID: "command-1",
				Success:   false,
				ErrorCode: strings.Repeat("x", 129),
			},
		},
		{
			name: "message too long",
			result: CommandResult{
				CommandID: "command-1",
				Success:   false,
				ErrorCode: "FAILED",
				Message:   strings.Repeat("x", 1025),
			},
		},
		{
			name: "unserializable result",
			result: CommandResult{
				CommandID: "command-1",
				Success:   true,
				Result: map[string]any{
					"invalid": make(chan int),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Validate()
			if !errors.Is(
				err,
				ErrInvalidCommandResponse,
			) {
				t.Fatalf(
					"expected validation error, got %v",
					err,
				)
			}
		})
	}
}

func TestCommandResponseJSON(t *testing.T) {
	accepted := CommandAccepted{
		CommandID: "command-1",
	}

	encoded, err := json.Marshal(accepted)
	if err != nil {
		t.Fatal(err)
	}

	if string(encoded) != `{"commandId":"command-1"}` {
		t.Fatalf(
			"unexpected accepted JSON: %s",
			encoded,
		)
	}

	result := CommandResult{
		CommandID: "command-1",
		Success:   true,
	}

	encoded, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	if string(encoded) !=
		`{"commandId":"command-1","success":true}` {
		t.Fatalf(
			"unexpected result JSON: %s",
			encoded,
		)
	}
}

func TestCommandResultMetadata(t *testing.T) {
	result := CommandResult{
		CommandID: "command-1",
		Success:   true,
		Result: map[string]any{
			"state": "running",
		},
	}

	if err := result.Validate(); err != nil {
		t.Fatalf("valid metadata rejected: %v", err)
	}
}
