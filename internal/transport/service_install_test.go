package transport

import (
	"encoding/json"
	"strings"
	"testing"
)

func validServiceInstallJSON() string {
	return `{
		"formatVersion": 1,
		"serviceId": "service-1",
		"jobId": "job-1",
		"nodeId": "node-1",
		"egg": {
			"id": "egg-1",
			"key": "nodejs-discord-bot",
			"version": "1.0.0"
		},
		"runtime": {
			"image": "node:22-bookworm-slim",
			"startup": "node index.js"
		},
		"resources": {
			"memoryMb": 512,
			"cpuPercent": 10,
			"diskMb": 512
		},
		"variables": [
			{
				"key": "BOT_TOKEN",
				"required": true
			}
		]
	}`
}

func TestDecodeServiceInstallPayloadValid(t *testing.T) {
	payload, err := DecodeServiceInstallPayload(
		[]byte(validServiceInstallJSON()),
	)
	if err != nil {
		t.Fatalf("expected valid payload: %v", err)
	}

	if payload.ServiceID != "service-1" ||
		payload.JobID != "job-1" ||
		payload.NodeID != "node-1" {
		t.Fatal("unexpected installation identifiers")
	}

	if len(payload.Variables) != 1 ||
		payload.Variables[0].Key != "BOT_TOKEN" {
		t.Fatal("unexpected variable metadata")
	}
}

func TestDecodeServiceInstallPayloadRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		edit func(map[string]any)
	}{
		{
			name: "unsupported version",
			edit: func(p map[string]any) {
				p["formatVersion"] = 2
			},
		},
		{
			name: "missing service ID",
			edit: func(p map[string]any) {
				delete(p, "serviceId")
			},
		},
		{
			name: "unknown top-level field",
			edit: func(p map[string]any) {
				p["shellCommand"] = "unexpected"
			},
		},
		{
			name: "unknown runtime field",
			edit: func(p map[string]any) {
				p["runtime"].(map[string]any)["privileged"] = true
			},
		},
		{
			name: "invalid memory",
			edit: func(p map[string]any) {
				p["resources"].(map[string]any)["memoryMb"] = -1
			},
		},
		{
			name: "CPU exceeds 100",
			edit: func(p map[string]any) {
				p["resources"].(map[string]any)["cpuPercent"] = 101
			},
		},
		{
			name: "secret value",
			edit: func(p map[string]any) {
				variables := p["variables"].([]any)
				variables[0].(map[string]any)["value"] = "secret"
			},
		},
		{
			name: "secret metadata field",
			edit: func(p map[string]any) {
				variables := p["variables"].([]any)
				variables[0].(map[string]any)["secret"] = true
			},
		},
		{
			name: "duplicate variable key",
			edit: func(p map[string]any) {
				p["variables"] = []any{
					map[string]any{"key": "TOKEN", "required": true},
					map[string]any{"key": "TOKEN", "required": false},
				}
			},
		},
		{
			name: "missing variables",
			edit: func(p map[string]any) {
				delete(p, "variables")
			},
		},
		{
			name: "null variables",
			edit: func(p map[string]any) {
				p["variables"] = nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal(
				[]byte(validServiceInstallJSON()),
				&payload,
			); err != nil {
				t.Fatal(err)
			}

			tt.edit(payload)

			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := DecodeServiceInstallPayload(data); err == nil {
				t.Fatal("expected payload rejection")
			}
		})
	}
}

func TestDecodeServiceInstallPayloadRejectsTrailingJSON(t *testing.T) {
	data := validServiceInstallJSON() + ` {"extra":true}`

	if _, err := DecodeServiceInstallPayload([]byte(data)); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
}

func TestDecodeServiceInstallPayloadRejectsMalformedJSON(t *testing.T) {
	if _, err := DecodeServiceInstallPayload([]byte(`{`)); err == nil {
		t.Fatal("expected malformed JSON rejection")
	}
}

func TestDecodeServiceInstallPayloadAllowsEmptyVariables(t *testing.T) {
	data := strings.Replace(
		validServiceInstallJSON(),
		`"variables": [
			{
				"key": "BOT_TOKEN",
				"required": true
			}
		]`,
		`"variables": []`,
		1,
	)

	payload, err := DecodeServiceInstallPayload([]byte(data))
	if err != nil {
		t.Fatalf("expected valid empty variables: %v", err)
	}

	if len(payload.Variables) != 0 {
		t.Fatal("expected no variables")
	}
}
