package protocol

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestPanelServiceEnvironmentInteroperability(t *testing.T) {
	data, err := os.ReadFile("testdata/agent-environment-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Token    string             `json:"token"`
		Binding  EnvironmentBinding `json:"binding"`
		Envelope json.RawMessage    `json:"envelope"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	values, err := OpenServiceEnvironment(fixture.Envelope, fixture.Token, fixture.Binding, map[string]bool{"TOKEN": true}, now)
	if err != nil || values["TOKEN"] != "fixture-only" {
		t.Fatal("Panel envelope could not be authenticated")
	}
	wrong := fixture.Binding
	wrong.ServiceID = "other-service"
	if _, err := OpenServiceEnvironment(fixture.Envelope, fixture.Token, wrong, map[string]bool{"TOKEN": true}, now); err == nil {
		t.Fatal("accepted another service binding")
	}
	if _, err := OpenServiceEnvironment(fixture.Envelope, fixture.Token, fixture.Binding, map[string]bool{"TOKEN": true}, now.Add(time.Minute)); err == nil {
		t.Fatal("accepted expired environment")
	}
	if _, err := OpenServiceEnvironment(fixture.Envelope, fixture.Token, fixture.Binding, map[string]bool{}, now); err == nil {
		t.Fatal("accepted undeclared variable")
	}
	if _, err := OpenServiceEnvironment(fixture.Envelope, "wrong", fixture.Binding, map[string]bool{"TOKEN": true}, now); err == nil {
		t.Fatal("accepted wrong token")
	}
}
