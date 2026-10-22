package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeStrictRejectsUnknownFields(t *testing.T) {
	var welcome Welcome

	err := decodeStrict([]byte(`{
		"nodeId": "node-1",
		"heartbeatIntervalSeconds": 30,
		"unexpected": true
	}`), &welcome)

	if err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

func TestDecodeStrictRejectsMultipleValues(t *testing.T) {
	var welcome Welcome

	err := decodeStrict([]byte(`{} {}`), &welcome)
	if err == nil {
		t.Fatal("expected multiple JSON values rejection")
	}
}

func TestHeartbeatACKValidation(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		valid   bool
	}{
		{"empty object", `{}`, true},
		{"null", `null`, false},
		{"array", `[]`, false},
		{"nonempty object", `{"ok":true}`, false},
		{"invalid JSON", `{`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHeartbeatACK(json.RawMessage(tt.payload))
			if (err == nil) != tt.valid {
				t.Fatalf("payload %s: unexpected error %v", tt.payload, err)
			}
		})
	}
}

func TestValidTimestamp(t *testing.T) {
	valid := []string{
		"2026-10-22T10:30:00Z",
		"2026-10-22T10:30:00.123Z",
	}

	for _, value := range valid {
		if !validTimestamp(value) {
			t.Errorf("expected valid timestamp: %s", value)
		}
	}

	invalid := []string{
		"",
		"2026-10-22",
		"2026-10-22T10:30:00+02:00",
		"not-a-date",
	}

	for _, value := range invalid {
		if validTimestamp(value) {
			t.Errorf("expected invalid timestamp: %s", value)
		}
	}
}

func TestHeartbeatRejectsInvalidDockerVersion(t *testing.T) {
	invalid := "   "

	err := (&Session{}).SendHeartbeat(
		t.Context(),
		Heartbeat{
			Docker: DockerStatus{
				Version: &invalid,
			},
		},
	)

	if err == nil || !strings.Contains(err.Error(), "Docker version") {
		t.Fatalf("expected Docker version validation error, got %v", err)
	}
}
