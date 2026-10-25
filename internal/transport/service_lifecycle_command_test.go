package transport

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tyravex/agent/internal/commands"
)

func TestDecodeServiceLifecycleCommandValid(t *testing.T) {
	operations := []commands.Operation{
		commands.ServiceStart,
		commands.ServiceStop,
		commands.ServiceRestart,
	}

	for _, operation := range operations {
		t.Run(string(operation), func(t *testing.T) {
			data := []byte(`{
				"commandId": "command-123",
				"operation": "` + string(operation) + `",
				"serviceId": "service-456",
				"parameters": {}
			}`)

			decoded, err := DecodeServiceLifecycleCommand(
				data,
			)
			if err != nil {
				t.Fatalf(
					"decode failed: %v",
					err,
				)
			}

			if decoded.Operation != operation {
				t.Fatalf(
					"unexpected operation: %q",
					decoded.Operation,
				)
			}

			command, err := decoded.DispatcherCommand()
			if err != nil {
				t.Fatalf(
					"conversion failed: %v",
					err,
				)
			}

			if command.ID != "command-123" ||
				command.ServiceID != "service-456" ||
				command.Operation != operation {
				t.Fatalf(
					"unexpected dispatcher command: %+v",
					command,
				)
			}
		})
	}
}

func TestDecodeServiceLifecycleCommandRejectsInvalidInput(
	t *testing.T,
) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "missing command ID",
			data: `{
				"operation":"service.start",
				"serviceId":"service-1",
				"parameters":{}
			}`,
		},
		{
			name: "empty command ID",
			data: `{
				"commandId":" ",
				"operation":"service.start",
				"serviceId":"service-1",
				"parameters":{}
			}`,
		},
		{
			name: "command ID too long",
			data: `{
				"commandId":"` + strings.Repeat("x", 129) + `",
				"operation":"service.start",
				"serviceId":"service-1",
				"parameters":{}
			}`,
		},
		{
			name: "missing service ID",
			data: `{
				"commandId":"command-1",
				"operation":"service.start",
				"parameters":{}
			}`,
		},
		{
			name: "unsupported operation",
			data: `{
				"commandId":"command-1",
				"operation":"service.delete",
				"serviceId":"service-1",
				"parameters":{}
			}`,
		},
		{
			name: "installation operation",
			data: `{
				"commandId":"command-1",
				"operation":"service.install",
				"serviceId":"service-1",
				"parameters":{}
			}`,
		},
		{
			name: "missing parameters",
			data: `{
				"commandId":"command-1",
				"operation":"service.start",
				"serviceId":"service-1"
			}`,
		},
		{
			name: "null parameters",
			data: `{
				"commandId":"command-1",
				"operation":"service.start",
				"serviceId":"service-1",
				"parameters":null
			}`,
		},
		{
			name: "nonempty parameters",
			data: `{
				"commandId":"command-1",
				"operation":"service.start",
				"serviceId":"service-1",
				"parameters":{"force":true}
			}`,
		},
		{
			name: "array parameters",
			data: `{
				"commandId":"command-1",
				"operation":"service.start",
				"serviceId":"service-1",
				"parameters":[]
			}`,
		},
		{
			name: "unknown field",
			data: `{
				"commandId":"command-1",
				"operation":"service.start",
				"serviceId":"service-1",
				"parameters":{},
				"nodeId":"other-node"
			}`,
		},
		{
			name: "duplicate command ID",
			data: `{
				"commandId":"command-1",
				"commandId":"command-2",
				"operation":"service.start",
				"serviceId":"service-1",
				"parameters":{}
			}`,
		},
		{
			name: "duplicate nested key",
			data: `{
				"commandId":"command-1",
				"operation":"service.start",
				"serviceId":"service-1",
				"parameters":{"force":true,"force":false}
			}`,
		},
		{
			name: "trailing JSON",
			data: `{
				"commandId":"command-1",
				"operation":"service.start",
				"serviceId":"service-1",
				"parameters":{}
			} {}`,
		},
		{
			name: "malformed JSON",
			data: `{"commandId":`,
		},
		{
			name: "null command",
			data: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeServiceLifecycleCommand(
				[]byte(tt.data),
			)

			if !errors.Is(
				err,
				ErrInvalidServiceLifecycleCommand,
			) {
				t.Fatalf(
					"expected lifecycle validation error, got %v",
					err,
				)
			}
		})
	}
}

func TestServiceLifecycleCommandRejectsInvalidConversion(
	t *testing.T,
) {
	command := ServiceLifecycleCommand{
		CommandID: "command-1",
		Operation: commands.ServiceStart,
		ServiceID: "service-1",
	}

	_, err := command.DispatcherCommand()
	if !errors.Is(
		err,
		ErrInvalidServiceLifecycleCommand,
	) {
		t.Fatalf(
			"expected validation error, got %v",
			err,
		)
	}
}
