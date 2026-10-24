package transport

import (
	"encoding/json"
	"testing"
)

func validServiceInstallCommandJSON(t *testing.T) []byte {
	t.Helper()

	var parameters map[string]any
	if err := json.Unmarshal(
		[]byte(validServiceInstallJSON()),
		&parameters,
	); err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(map[string]any{
		"commandId":  "command-1",
		"operation":  "service.install",
		"serviceId":  "service-1",
		"parameters": parameters,
	})
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestDecodeServiceInstallCommandValid(t *testing.T) {
	command, err := DecodeServiceInstallCommand(
		validServiceInstallCommandJSON(t),
	)
	if err != nil {
		t.Fatalf("expected valid command: %v", err)
	}

	if command.CommandID != "command-1" {
		t.Fatal("unexpected command ID")
	}

	if command.ServiceID != "service-1" {
		t.Fatal("unexpected service ID")
	}

	if command.Install.JobID != "job-1" {
		t.Fatal("unexpected installation job ID")
	}
}

func TestDecodeServiceInstallCommandRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		edit func(map[string]any)
	}{
		{
			name: "missing command ID",
			edit: func(p map[string]any) {
				delete(p, "commandId")
			},
		},
		{
			name: "empty command ID",
			edit: func(p map[string]any) {
				p["commandId"] = " "
			},
		},
		{
			name: "unsupported operation",
			edit: func(p map[string]any) {
				p["operation"] = "server.start"
			},
		},
		{
			name: "missing service ID",
			edit: func(p map[string]any) {
				delete(p, "serviceId")
			},
		},
		{
			name: "service ID mismatch",
			edit: func(p map[string]any) {
				p["serviceId"] = "another-service"
			},
		},
		{
			name: "missing parameters",
			edit: func(p map[string]any) {
				delete(p, "parameters")
			},
		},
		{
			name: "unknown command field",
			edit: func(p map[string]any) {
				p["shellCommand"] = "unexpected"
			},
		},
		{
			name: "legacy server ID",
			edit: func(p map[string]any) {
				p["serverId"] = "legacy-server"
			},
		},
		{
			name: "secret value in parameters",
			edit: func(p map[string]any) {
				parameters := p["parameters"].(map[string]any)
				variables := parameters["variables"].([]any)
				variables[0].(map[string]any)["value"] = "secret"
			},
		},
		{
			name: "invalid installation version",
			edit: func(p map[string]any) {
				p["parameters"].(map[string]any)["formatVersion"] = 2
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal(
				validServiceInstallCommandJSON(t),
				&payload,
			); err != nil {
				t.Fatal(err)
			}

			tt.edit(payload)

			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := DecodeServiceInstallCommand(data); err == nil {
				t.Fatal("expected command rejection")
			}
		})
	}
}

func TestDecodeServiceInstallCommandRejectsTrailingJSON(t *testing.T) {
	data := append(
		validServiceInstallCommandJSON(t),
		[]byte(` {"extra":true}`)...,
	)

	if _, err := DecodeServiceInstallCommand(data); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
}

func TestDecodeServiceInstallCommandRejectsMalformedJSON(t *testing.T) {
	if _, err := DecodeServiceInstallCommand([]byte(`{`)); err == nil {
		t.Fatal("expected malformed JSON rejection")
	}
}
