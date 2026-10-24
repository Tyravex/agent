package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	serviceInstallFormatVersion = 1
	maxInstallVariables         = 64
	maxSafeJSONInteger          = int64(9007199254740991)
)

type ServiceInstallEgg struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Version string `json:"version"`
}

type ServiceInstallRuntime struct {
	Image   string `json:"image"`
	Startup string `json:"startup"`
}

type ServiceInstallResources struct {
	MemoryMB   int64 `json:"memoryMb"`
	CPUPercent int64 `json:"cpuPercent"`
	DiskMB     int64 `json:"diskMb"`
}

type ServiceInstallVariable struct {
	Key         string  `json:"key"`
	Description *string `json:"description,omitempty"`
	Required    bool    `json:"required"`
}

type ServiceInstallPayload struct {
	FormatVersion int                      `json:"formatVersion"`
	ServiceID     string                   `json:"serviceId"`
	JobID         string                   `json:"jobId"`
	NodeID        string                   `json:"nodeId"`
	Egg           ServiceInstallEgg        `json:"egg"`
	Runtime       ServiceInstallRuntime    `json:"runtime"`
	Resources     ServiceInstallResources  `json:"resources"`
	Variables     []ServiceInstallVariable `json:"variables"`
}

func DecodeServiceInstallPayload(data []byte) (ServiceInstallPayload, error) {
	var payload ServiceInstallPayload

	if err := rejectDuplicateJSONKeys(data); err != nil {
		return ServiceInstallPayload{}, fmt.Errorf(
			"invalid installation JSON keys: %w",
			err,
		)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&payload); err != nil {
		return ServiceInstallPayload{}, fmt.Errorf(
			"invalid service installation JSON: %w",
			err,
		)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ServiceInstallPayload{}, errors.New(
			"service installation payload contains trailing data",
		)
	}

	if err := ValidateServiceInstallPayload(payload); err != nil {
		return ServiceInstallPayload{}, err
	}

	return payload, nil
}

func ValidateServiceInstallPayload(payload ServiceInstallPayload) error {
	if payload.FormatVersion != serviceInstallFormatVersion {
		return errors.New("unsupported installation format version")
	}

	if !validInstallString(payload.ServiceID, 128) ||
		!validInstallString(payload.JobID, 128) ||
		!validInstallString(payload.NodeID, 128) {
		return errors.New("invalid installation identifiers")
	}

	if !validInstallString(payload.Egg.ID, 128) ||
		!validInstallString(payload.Egg.Key, 128) ||
		!validInstallString(payload.Egg.Version, 64) {
		return errors.New("invalid installation egg")
	}

	if !validInstallString(payload.Runtime.Image, 512) ||
		!validInstallString(payload.Runtime.Startup, 2048) {
		return errors.New("invalid installation runtime")
	}

	resources := payload.Resources

	if !validInstallInteger(resources.MemoryMB) ||
		!validInstallInteger(resources.DiskMB) ||
		!validInstallInteger(resources.CPUPercent) ||
		resources.CPUPercent > 100 {
		return errors.New("invalid installation resources")
	}

	if payload.Variables == nil ||
		len(payload.Variables) > maxInstallVariables {
		return errors.New("invalid installation variables")
	}

	seen := make(map[string]struct{}, len(payload.Variables))

	for _, variable := range payload.Variables {
		if !validInstallString(variable.Key, 128) {
			return errors.New("invalid installation variable key")
		}

		if variable.Description != nil &&
			len(*variable.Description) > 1024 {
			return errors.New("invalid installation variable description")
		}

		if _, exists := seen[variable.Key]; exists {
			return errors.New("duplicate installation variable key")
		}

		seen[variable.Key] = struct{}{}
	}

	return nil
}

func validInstallString(value string, maxLength int) bool {
	return strings.TrimSpace(value) != "" &&
		len(value) <= maxLength
}

func validInstallInteger(value int64) bool {
	return value > 0 && value <= maxSafeJSONInteger
}
