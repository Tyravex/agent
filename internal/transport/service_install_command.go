package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const serviceInstallOperation = "service.install"

type ServiceInstallCommand struct {
	CommandID string
	ServiceID string
	Install   ServiceInstallPayload
}

func DecodeServiceInstallCommand(
	data []byte,
) (ServiceInstallCommand, error) {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return ServiceInstallCommand{}, fmt.Errorf(
			"invalid installation command JSON keys: %w",
			err,
		)
	}
	// Decode into raw fields first to preserve strict validation
	// of both the command wrapper and installation parameters.
	var raw struct {
		CommandID  string          `json:"commandId"`
		Operation  string          `json:"operation"`
		ServiceID  string          `json:"serviceId"`
		Parameters json.RawMessage `json:"parameters"`
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&raw); err != nil {
		return ServiceInstallCommand{}, fmt.Errorf(
			"invalid service installation command: %w",
			err,
		)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ServiceInstallCommand{}, errors.New(
			"service installation command contains trailing data",
		)
	}

	if !validInstallCommandID(raw.CommandID) {
		return ServiceInstallCommand{}, errors.New(
			"invalid installation command ID",
		)
	}

	if raw.Operation != serviceInstallOperation {
		return ServiceInstallCommand{}, errors.New(
			"unsupported installation operation",
		)
	}

	if !validInstallString(raw.ServiceID, 128) {
		return ServiceInstallCommand{}, errors.New(
			"invalid installation service ID",
		)
	}

	if len(raw.Parameters) == 0 {
		return ServiceInstallCommand{}, errors.New(
			"missing installation parameters",
		)
	}

	install, err := DecodeServiceInstallPayload(raw.Parameters)
	if err != nil {
		return ServiceInstallCommand{}, fmt.Errorf(
			"invalid installation parameters: %w",
			err,
		)
	}

	if install.ServiceID != raw.ServiceID {
		return ServiceInstallCommand{}, errors.New(
			"installation service ID mismatch",
		)
	}

	return ServiceInstallCommand{
		CommandID: raw.CommandID,
		ServiceID: raw.ServiceID,
		Install:   install,
	}, nil
}

func validInstallCommandID(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 128
}
