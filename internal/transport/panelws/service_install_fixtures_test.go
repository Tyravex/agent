package panelws

import (
	"encoding/json"
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
