package transport

import (
	"testing"
)

func TestOwnershipWelcomeSessionIDValidation(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{
			name:  "valid random identifier",
			value: "e4f19a7c6d2b4058b3f1a90c7e6d5421",
			valid: true,
		},
		{
			name:  "valid URL safe identifier",
			value: "session_A1b2C3d4E5f6",
			valid: true,
		},
		{
			name:  "empty",
			value: "",
			valid: false,
		},
		{
			name:  "too short",
			value: "session-1",
			valid: false,
		},
		{
			name:  "whitespace",
			value: " session_A1b2C3d4E5f6",
			valid: false,
		},
		{
			name:  "invalid separator",
			value: "session.A1b2C3d4E5f6",
			valid: false,
		},
		{
			name:  "too long",
			value: string(make([]byte, 129)),
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validOwnershipSessionID(tt.value)
			if got != tt.valid {
				t.Fatalf(
					"validOwnershipSessionID(%q) = %v, want %v",
					tt.value,
					got,
					tt.valid,
				)
			}
		})
	}
}
