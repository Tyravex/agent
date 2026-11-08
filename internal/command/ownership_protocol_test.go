package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func validOwnershipProtocolJSON() []byte {
	return []byte(`{
		"type": "ownership.snapshot",
		"session_id": "session-1",
		"node_id": "node-1",
		"revision": 10,
		"ttl_seconds": 60,
		"assignments": [
			{
				"service_id": "minecraft-1",
				"node_id": "node-1",
				"enabled": true
			}
		]
	}`)
}

func TestOwnershipProtocolDecodeValid(t *testing.T) {
	message, err := DecodeOwnershipSnapshotMessage(
		validOwnershipProtocolJSON(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if message.Type != OwnershipSnapshotMessageType ||
		message.Revision != 10 ||
		message.TTLSeconds != 60 ||
		len(message.Assignments) != 1 {
		t.Fatalf("unexpected message: %+v", message)
	}
}

func TestOwnershipProtocolRejectsUnknownField(t *testing.T) {
	data := []byte(`{
		"type": "ownership.snapshot",
		"session_id": "session-1",
		"node_id": "node-1",
		"revision": 1,
		"ttl_seconds": 60,
		"assignments": [],
		"generation": 999
	}`)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}
}

func TestOwnershipProtocolRejectsTrailingJSON(t *testing.T) {
	data := append(
		validOwnershipProtocolJSON(),
		[]byte(` {}`)...,
	)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}
}

func TestOwnershipProtocolRejectsWrongType(t *testing.T) {
	data := []byte(`{
		"type": "ownership.delete",
		"session_id": "session-1",
		"node_id": "node-1",
		"revision": 1,
		"ttl_seconds": 60,
		"assignments": []
	}`)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrOwnershipMessageType) {
		t.Fatalf("expected wrong type, got %v", err)
	}
}

func TestOwnershipProtocolRejectsOversizedMessage(t *testing.T) {
	data := []byte(
		strings.Repeat(" ", maxOwnershipMessageBytes+1),
	)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrOwnershipMessageTooLarge) {
		t.Fatalf("expected oversized message, got %v", err)
	}
}

func TestOwnershipProtocolRejectsExcessiveTTL(t *testing.T) {
	data := []byte(`{
		"type": "ownership.snapshot",
		"session_id": "session-1",
		"node_id": "node-1",
		"revision": 1,
		"ttl_seconds": 301,
		"assignments": []
	}`)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrOwnershipSnapshotExpired) {
		t.Fatalf("expected expired snapshot, got %v", err)
	}
}

func TestOwnershipProtocolRejectsWrongAssignmentNode(
	t *testing.T,
) {
	data := []byte(`{
		"type": "ownership.snapshot",
		"session_id": "session-1",
		"node_id": "node-1",
		"revision": 1,
		"ttl_seconds": 60,
		"assignments": [
			{
				"service_id": "minecraft-1",
				"node_id": "node-2",
				"enabled": true
			}
		]
	}`)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}
}

func TestOwnershipProtocolRejectsInvalidServiceID(
	t *testing.T,
) {
	data := []byte(`{
		"type": "ownership.snapshot",
		"session_id": "session-1",
		"node_id": "node-1",
		"revision": 1,
		"ttl_seconds": 60,
		"assignments": [
			{
				"service_id": "../escape",
				"node_id": "node-1",
				"enabled": true
			}
		]
	}`)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}
}

func TestOwnershipProtocolApplyValid(t *testing.T) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx,
		"session-1",
		1,
	); err != nil {
		t.Fatal(err)
	}

	if err := ApplyOwnershipSnapshotMessage(
		ctx,
		manager,
		1,
		validOwnershipProtocolJSON(),
	); err != nil {
		t.Fatal(err)
	}

	status := manager.Status()

	if status.State != OwnershipReady ||
		status.Revision != 10 ||
		status.Generation != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}

	ownership, err := manager.Lookup(
		ctx,
		"minecraft-1",
	)
	if err != nil {
		t.Fatal(err)
	}

	if !ownership.Enabled ||
		ownership.NodeID != "node-1" {
		t.Fatalf("unexpected ownership: %+v", ownership)
	}
}

func TestOwnershipProtocolRejectsOldGeneration(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx,
		"session-1",
		2,
	); err != nil {
		t.Fatal(err)
	}

	err := ApplyOwnershipSnapshotMessage(
		ctx,
		manager,
		1,
		validOwnershipProtocolJSON(),
	)

	if !errors.Is(err, ErrOwnershipGenerationMismatch) {
		t.Fatalf("expected generation mismatch, got %v", err)
	}

	if got := manager.Status().State; got != OwnershipNotReady {
		t.Fatalf("expected NOT_READY, got %s", got)
	}
}

func TestOwnershipProtocolRejectsWrongNode(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-2")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx,
		"session-1",
		1,
	); err != nil {
		t.Fatal(err)
	}

	err := ApplyOwnershipSnapshotMessage(
		ctx,
		manager,
		1,
		validOwnershipProtocolJSON(),
	)

	if !errors.Is(err, ErrInvalidOwnershipSnapshot) {
		t.Fatalf("expected invalid snapshot, got %v", err)
	}
}

func TestOwnershipProtocolRejectsDuplicateTopLevelKey(
	t *testing.T,
) {
	data := []byte(`{
		"type": "ownership.snapshot",
		"type": "ownership.snapshot",
		"session_id": "session-1",
		"node_id": "node-1",
		"revision": 1,
		"ttl_seconds": 60,
		"assignments": []
	}`)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}
}

func TestOwnershipProtocolRejectsDuplicateAssignmentKey(
	t *testing.T,
) {
	data := []byte(`{
		"type": "ownership.snapshot",
		"session_id": "session-1",
		"node_id": "node-1",
		"revision": 1,
		"ttl_seconds": 60,
		"assignments": [
			{
				"service_id": "minecraft-1",
				"service_id": "minecraft-2",
				"node_id": "node-1",
				"enabled": true
			}
		]
	}`)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}
}

func TestOwnershipProtocolRejectsMissingFields(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "missing type",
			data: `{
				"session_id":"session-1",
				"node_id":"node-1",
				"revision":1,
				"ttl_seconds":60,
				"assignments":[]
			}`,
		},
		{
			name: "missing revision",
			data: `{
				"type":"ownership.snapshot",
				"session_id":"session-1",
				"node_id":"node-1",
				"ttl_seconds":60,
				"assignments":[]
			}`,
		},
		{
			name: "missing assignments",
			data: `{
				"type":"ownership.snapshot",
				"session_id":"session-1",
				"node_id":"node-1",
				"revision":1,
				"ttl_seconds":60
			}`,
		},
		{
			name: "missing enabled",
			data: `{
				"type":"ownership.snapshot",
				"session_id":"session-1",
				"node_id":"node-1",
				"revision":1,
				"ttl_seconds":60,
				"assignments":[{
					"service_id":"minecraft-1",
					"node_id":"node-1"
				}]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeOwnershipSnapshotMessage(
				[]byte(tt.data),
			)

			if !errors.Is(err, ErrInvalidOwnershipMessage) {
				t.Fatalf("expected invalid message, got %v", err)
			}
		})
	}
}

func TestOwnershipProtocolRejectsNullFields(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "null assignments",
			data: `{
				"type":"ownership.snapshot",
				"session_id":"session-1",
				"node_id":"node-1",
				"revision":1,
				"ttl_seconds":60,
				"assignments":null
			}`,
		},
		{
			name: "null enabled",
			data: `{
				"type":"ownership.snapshot",
				"session_id":"session-1",
				"node_id":"node-1",
				"revision":1,
				"ttl_seconds":60,
				"assignments":[{
					"service_id":"minecraft-1",
					"node_id":"node-1",
					"enabled":null
				}]
			}`,
		},
		{
			name: "null assignment object",
			data: `{
				"type":"ownership.snapshot",
				"session_id":"session-1",
				"node_id":"node-1",
				"revision":1,
				"ttl_seconds":60,
				"assignments":[null]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeOwnershipSnapshotMessage(
				[]byte(tt.data),
			)

			if !errors.Is(err, ErrInvalidOwnershipMessage) {
				t.Fatalf("expected invalid message, got %v", err)
			}
		})
	}
}

func TestOwnershipProtocolRejectsIncorrectFieldCasing(
	t *testing.T,
) {
	data := []byte(`{
		"type":"ownership.snapshot",
		"session_id":"session-1",
		"node_id":"node-1",
		"Revision":1,
		"ttl_seconds":60,
		"assignments":[]
	}`)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}
}

func TestOwnershipProtocolRejectsDuplicateServiceIDs(
	t *testing.T,
) {
	data := []byte(`{
		"type":"ownership.snapshot",
		"session_id":"session-1",
		"node_id":"node-1",
		"revision":1,
		"ttl_seconds":60,
		"assignments":[
			{
				"service_id":"minecraft-1",
				"node_id":"node-1",
				"enabled":true
			},
			{
				"service_id":"minecraft-1",
				"node_id":"node-1",
				"enabled":false
			}
		]
	}`)

	_, err := DecodeOwnershipSnapshotMessage(data)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}
}

func TestOwnershipProtocolRejectsTooManyAssignments(
	t *testing.T,
) {
	var builder strings.Builder

	builder.WriteString(`{
		"type":"ownership.snapshot",
		"session_id":"session-1",
		"node_id":"node-1",
		"revision":1,
		"ttl_seconds":60,
		"assignments":[
	`)

	for i := 0; i <= maxOwnershipAssignments; i++ {
		if i > 0 {
			builder.WriteByte(',')
		}

		fmt.Fprintf(
			&builder,
			`{"service_id":"s%d","node_id":"node-1","enabled":true}`,
			i,
		)
	}

	builder.WriteString(`]}`)

	_, err := DecodeOwnershipSnapshotMessage(
		[]byte(builder.String()),
	)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}
}

func TestOwnershipProtocolInvalidMessageDoesNotChangeState(
	t *testing.T,
) {
	manager := NewOwnershipSyncManager("node-1")
	ctx := context.Background()

	if err := manager.BeginSessionWithGeneration(
		ctx,
		"session-1",
		1,
	); err != nil {
		t.Fatal(err)
	}

	if err := ApplyOwnershipSnapshotMessage(
		ctx,
		manager,
		1,
		validOwnershipProtocolJSON(),
	); err != nil {
		t.Fatal(err)
	}

	before := manager.Status()

	invalid := []byte(`{
		"type":"ownership.snapshot",
		"session_id":"session-1",
		"node_id":"node-1",
		"revision":999,
		"ttl_seconds":60,
		"assignments":[
			{
				"service_id":"minecraft-1",
				"node_id":"node-1",
				"enabled":true
			},
			{
				"service_id":"minecraft-1",
				"node_id":"node-1",
				"enabled":false
			}
		]
	}`)

	err := ApplyOwnershipSnapshotMessage(
		ctx,
		manager,
		1,
		invalid,
	)

	if !errors.Is(err, ErrInvalidOwnershipMessage) {
		t.Fatalf("expected invalid message, got %v", err)
	}

	after := manager.Status()

	if after.State != before.State ||
		after.Revision != before.Revision ||
		after.HighestRevision != before.HighestRevision ||
		after.Generation != before.Generation ||
		!after.ExpiresAt.Equal(before.ExpiresAt) {
		t.Fatalf(
			"invalid message changed state: before=%+v after=%+v",
			before,
			after,
		)
	}

	ownership, err := manager.Lookup(ctx, "minecraft-1")
	if err != nil {
		t.Fatal(err)
	}

	if !ownership.Enabled {
		t.Fatal("invalid message revoked valid ownership")
	}
}
