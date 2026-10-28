package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	OwnershipSnapshotMessageType = "ownership.snapshot"

	maxOwnershipMessageBytes = 1 << 20
	maxOwnershipAssignments  = 4096
)

var (
	ErrInvalidOwnershipMessage = errors.New(
		"invalid ownership synchronization message",
	)
	ErrOwnershipMessageTooLarge = errors.New(
		"ownership synchronization message too large",
	)
	ErrOwnershipMessageType = errors.New(
		"unsupported ownership synchronization message type",
	)
)

type OwnershipSnapshotMessage struct {
	Type        string                       `json:"type"`
	SessionID   string                       `json:"session_id"`
	NodeID      string                       `json:"node_id"`
	Revision    uint64                       `json:"revision"`
	TTLSeconds  uint64                       `json:"ttl_seconds"`
	Assignments []OwnershipAssignmentMessage `json:"assignments"`
}

type OwnershipAssignmentMessage struct {
	ServiceID string `json:"service_id"`
	NodeID    string `json:"node_id"`
	Enabled   bool   `json:"enabled"`
}

// DecodeOwnershipSnapshotMessage decodes exactly one
// strictly validated ownership.snapshot JSON object.
//
// Every field is required. Unknown fields, duplicate keys,
// null values, incorrect key casing and trailing JSON
// are rejected.
//
// The input size is limited before decoding.
func DecodeOwnershipSnapshotMessage(
	data []byte,
) (OwnershipSnapshotMessage, error) {
	if len(data) > maxOwnershipMessageBytes {
		return OwnershipSnapshotMessage{},
			ErrOwnershipMessageTooLarge
	}

	decoder := json.NewDecoder(bytes.NewReader(data))

	raw, err := decodeStrictJSONObject(
		decoder,
		[]string{
			"type",
			"session_id",
			"node_id",
			"revision",
			"ttl_seconds",
			"assignments",
		},
	)
	if err != nil {
		return OwnershipSnapshotMessage{}, err
	}

	var trailing any

	if err := decoder.Decode(&trailing); err != io.EOF {
		return OwnershipSnapshotMessage{},
			ErrInvalidOwnershipMessage
	}

	var message OwnershipSnapshotMessage

	if err := decodeRequiredJSONField(
		raw,
		"type",
		&message.Type,
	); err != nil {
		return OwnershipSnapshotMessage{}, err
	}

	if err := decodeRequiredJSONField(
		raw,
		"session_id",
		&message.SessionID,
	); err != nil {
		return OwnershipSnapshotMessage{}, err
	}

	if err := decodeRequiredJSONField(
		raw,
		"node_id",
		&message.NodeID,
	); err != nil {
		return OwnershipSnapshotMessage{}, err
	}

	if err := decodeRequiredJSONField(
		raw,
		"revision",
		&message.Revision,
	); err != nil {
		return OwnershipSnapshotMessage{}, err
	}

	if err := decodeRequiredJSONField(
		raw,
		"ttl_seconds",
		&message.TTLSeconds,
	); err != nil {
		return OwnershipSnapshotMessage{}, err
	}

	assignmentsRaw := raw["assignments"]

	if bytes.Equal(
		bytes.TrimSpace(assignmentsRaw),
		[]byte("null"),
	) {
		return OwnershipSnapshotMessage{},
			ErrInvalidOwnershipMessage
	}

	var assignmentItems []json.RawMessage

	if err := json.Unmarshal(
		assignmentsRaw,
		&assignmentItems,
	); err != nil {
		return OwnershipSnapshotMessage{},
			fmt.Errorf(
				"%w: assignments: %v",
				ErrInvalidOwnershipMessage,
				err,
			)
	}

	if len(assignmentItems) > maxOwnershipAssignments {
		return OwnershipSnapshotMessage{},
			ErrInvalidOwnershipMessage
	}

	message.Assignments = make(
		[]OwnershipAssignmentMessage,
		0,
		len(assignmentItems),
	)

	for _, item := range assignmentItems {
		assignment, err := decodeOwnershipAssignment(item)
		if err != nil {
			return OwnershipSnapshotMessage{}, err
		}

		message.Assignments = append(
			message.Assignments,
			assignment,
		)
	}

	if err := validateOwnershipSnapshotMessage(
		message,
	); err != nil {
		return OwnershipSnapshotMessage{}, err
	}

	return message, nil
}

// ApplyOwnershipSnapshotMessage applies a validated Panel
// snapshot to the currently authenticated connection.
//
// The generation must be assigned by trusted Agent transport
// code and must not originate from the JSON payload.
func ApplyOwnershipSnapshotMessage(
	ctx context.Context,
	manager *OwnershipSyncManager,
	generation uint64,
	data []byte,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if manager == nil {
		return ErrAuthorizationUnavailable
	}

	message, err := DecodeOwnershipSnapshotMessage(data)
	if err != nil {
		return err
	}

	if message.NodeID != manager.nodeID {
		return ErrInvalidOwnershipSnapshot
	}

	assignments := make(
		[]ServiceOwnership,
		0,
		len(message.Assignments),
	)

	for _, assignment := range message.Assignments {
		assignments = append(
			assignments,
			ServiceOwnership{
				ServiceID: assignment.ServiceID,
				NodeID:    assignment.NodeID,
				Enabled:   assignment.Enabled,
			},
		)
	}

	snapshot := OwnershipSnapshot{
		NodeID:      message.NodeID,
		Revision:    message.Revision,
		Assignments: assignments,
	}

	return manager.ApplySnapshotWithGeneration(
		ctx,
		message.SessionID,
		generation,
		snapshot,
		time.Duration(message.TTLSeconds)*time.Second,
	)
}

// decodeStrictJSONObject reads a JSON object using the
// token API so duplicate keys cannot be silently accepted.
//
// Allowed keys must match exactly, including letter casing.
// Every allowed key is required.
func decodeStrictJSONObject(
	decoder *json.Decoder,
	allowedKeys []string,
) (map[string]json.RawMessage, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %v",
			ErrInvalidOwnershipMessage,
			err,
		)
	}

	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return nil, ErrInvalidOwnershipMessage
	}

	allowed := make(map[string]struct{}, len(allowedKeys))

	for _, key := range allowedKeys {
		allowed[key] = struct{}{}
	}

	fields := make(
		map[string]json.RawMessage,
		len(allowedKeys),
	)

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf(
				"%w: %v",
				ErrInvalidOwnershipMessage,
				err,
			)
		}

		key, ok := token.(string)
		if !ok {
			return nil, ErrInvalidOwnershipMessage
		}

		if _, exists := allowed[key]; !exists {
			return nil, ErrInvalidOwnershipMessage
		}

		if _, exists := fields[key]; exists {
			return nil, ErrInvalidOwnershipMessage
		}

		var value json.RawMessage

		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf(
				"%w: %v",
				ErrInvalidOwnershipMessage,
				err,
			)
		}

		if bytes.Equal(
			bytes.TrimSpace(value),
			[]byte("null"),
		) {
			return nil, ErrInvalidOwnershipMessage
		}

		fields[key] = value
	}

	token, err = decoder.Token()
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %v",
			ErrInvalidOwnershipMessage,
			err,
		)
	}

	delim, ok = token.(json.Delim)
	if !ok || delim != '}' {
		return nil, ErrInvalidOwnershipMessage
	}

	if len(fields) != len(allowedKeys) {
		return nil, ErrInvalidOwnershipMessage
	}

	return fields, nil
}

// decodeRequiredJSONField decodes a required field into
// its exact Go type.
//
// JSON null is rejected before unmarshalling.
func decodeRequiredJSONField(
	fields map[string]json.RawMessage,
	name string,
	target any,
) error {
	value, exists := fields[name]
	if !exists {
		return ErrInvalidOwnershipMessage
	}

	if bytes.Equal(
		bytes.TrimSpace(value),
		[]byte("null"),
	) {
		return ErrInvalidOwnershipMessage
	}

	if err := json.Unmarshal(value, target); err != nil {
		return fmt.Errorf(
			"%w: %s: %v",
			ErrInvalidOwnershipMessage,
			name,
			err,
		)
	}

	return nil
}

// decodeOwnershipAssignment strictly decodes one
// service assignment.
//
// The enabled field is required even when false.
func decodeOwnershipAssignment(
	data []byte,
) (OwnershipAssignmentMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	fields, err := decodeStrictJSONObject(
		decoder,
		[]string{
			"service_id",
			"node_id",
			"enabled",
		},
	)
	if err != nil {
		return OwnershipAssignmentMessage{}, err
	}

	var trailing any

	if err := decoder.Decode(&trailing); err != io.EOF {
		return OwnershipAssignmentMessage{},
			ErrInvalidOwnershipMessage
	}

	var assignment OwnershipAssignmentMessage

	if err := decodeRequiredJSONField(
		fields,
		"service_id",
		&assignment.ServiceID,
	); err != nil {
		return OwnershipAssignmentMessage{}, err
	}

	if err := decodeRequiredJSONField(
		fields,
		"node_id",
		&assignment.NodeID,
	); err != nil {
		return OwnershipAssignmentMessage{}, err
	}

	if err := decodeRequiredJSONField(
		fields,
		"enabled",
		&assignment.Enabled,
	); err != nil {
		return OwnershipAssignmentMessage{}, err
	}

	return assignment, nil
}

// validateOwnershipSnapshotMessage checks semantic
// constraints after strict JSON decoding.
func validateOwnershipSnapshotMessage(
	message OwnershipSnapshotMessage,
) error {
	if message.Type != OwnershipSnapshotMessageType {
		return ErrOwnershipMessageType
	}

	if !validOwnershipSessionID(message.SessionID) {
		return ErrInvalidOwnershipMessage
	}

	if message.NodeID == "" || message.Revision == 0 {
		return ErrInvalidOwnershipMessage
	}

	if message.TTLSeconds == 0 ||
		message.TTLSeconds > uint64(
			maxOwnershipSnapshotTTL/time.Second,
		) {
		return ErrOwnershipSnapshotExpired
	}

	if len(message.Assignments) > maxOwnershipAssignments {
		return ErrInvalidOwnershipMessage
	}

	seen := make(
		map[string]struct{},
		len(message.Assignments),
	)

	for _, assignment := range message.Assignments {
		if !authorizedServiceIDPattern.MatchString(
			assignment.ServiceID,
		) {
			return ErrInvalidOwnershipMessage
		}

		if assignment.NodeID != message.NodeID {
			return ErrInvalidOwnershipMessage
		}

		if _, exists := seen[assignment.ServiceID]; exists {
			return ErrInvalidOwnershipMessage
		}

		seen[assignment.ServiceID] = struct{}{}
	}

	return nil
}
