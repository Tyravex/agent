package transport

import (
	"strings"
	"testing"
)

func TestRejectDuplicateJSONKeys(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "simple object",
			input:   `{"id":"one","type":"test"}`,
			wantErr: false,
		},
		{
			name:    "same key in different objects",
			input:   `{"first":{"id":"one"},"second":{"id":"two"}}`,
			wantErr: false,
		},
		{
			name:    "same key in array elements",
			input:   `[{"key":"one"},{"key":"two"}]`,
			wantErr: false,
		},
		{
			name:    "duplicate top-level key",
			input:   `{"operation":"service.install","operation":"server.start"}`,
			wantErr: true,
		},
		{
			name:    "duplicate nested key",
			input:   `{"runtime":{"image":"node:22","image":"node:20"}}`,
			wantErr: true,
		},
		{
			name:    "duplicate key inside array",
			input:   `{"variables":[{"key":"TOKEN","key":"OTHER"}]}`,
			wantErr: true,
		},
		{
			name:    "multiple JSON values",
			input:   `{"id":"one"} {"id":"two"}`,
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			input:   `{"id":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectDuplicateJSONKeys([]byte(tt.input))

			if tt.wantErr && err == nil {
				t.Fatal("expected rejection")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected rejection: %v", err)
			}
		})
	}
}

func TestDecodeServiceInstallCommandRejectsDuplicateKeys(t *testing.T) {
	tests := []struct {
		name   string
		before string
		after  string
	}{
		{
			name:   "duplicate operation",
			before: `"operation":"service.install"`,
			after:  `"operation":"service.install","operation":"server.start"`,
		},
		{
			name:   "duplicate service ID",
			before: `"serviceId":"service-1"`,
			after:  `"serviceId":"service-1","serviceId":"service-2"`,
		},
		{
			name:   "duplicate runtime image",
			before: `"image":"node:22-bookworm-slim"`,
			after:  `"image":"node:22-bookworm-slim","image":"node:20"`,
		},
		{
			name:   "duplicate variable key",
			before: `"key":"BOT_TOKEN"`,
			after:  `"key":"BOT_TOKEN","key":"OTHER_TOKEN"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := string(validServiceInstallCommandJSON(t))

			if !strings.Contains(data, tt.before) {
				t.Fatalf("test field not found: %s", tt.before)
			}

			data = strings.Replace(data, tt.before, tt.after, 1)

			if _, err := DecodeServiceInstallCommand([]byte(data)); err == nil {
				t.Fatal("expected duplicate JSON key rejection")
			}
		})
	}
}

func TestDecodeServiceInstallPayloadRejectsDuplicateKeys(t *testing.T) {
	data := strings.Replace(
		validServiceInstallJSON(),
		`"image": "node:22-bookworm-slim"`,
		`"image": "node:22-bookworm-slim", "image": "node:20"`,
		1,
	)

	if _, err := DecodeServiceInstallPayload([]byte(data)); err == nil {
		t.Fatal("expected duplicate JSON key rejection")
	}
}
