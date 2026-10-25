# Persistent Command Registry

## Status

Introduced in Tyravex Agent v0.4.4 as an opt-in transport component.

The default `transport.NewClient(cfg)` uses an in-memory command ID registry. Persistent command ID history is available through `transport.NewClientWithPersistentRegistry(cfg, journalPath)`.

The production Agent runner does not yet enable lifecycle command execution over WebSocket. Connecting the persistent registry does not authorize Docker operations or enable command execution by itself.

## Purpose

The registry maintains an append-only JSONL journal of reserved command IDs.

A command ID is persisted and synchronized before the transport acknowledges and executes the associated lifecycle command.

Previously reserved IDs are rejected after the registry is reopened.

This prevents accidental command ID reuse. It does not provide exactly-once execution guarantees across process crashes, network failures or external side effects.

## Journal format

Each record contains one `commandId` field and ends with a newline.

Example:

```json
{"commandId":"example-command-1"}
```

The journal uses the canonical JSON representation produced by the Agent.

Malformed records, duplicate IDs, noncanonical JSON, oversized records and incomplete final records cause startup to fail closed.

The registry retains a maximum of 4096 command IDs. Automatic rotation, compaction and expiration are not implemented.

## Locking

The registry creates a companion `.lock` file using exclusive creation.

Only cooperating processes that respect this lock convention are prevented from opening the same journal concurrently.

The lock is released during normal registry shutdown, after all tracked WebSocket sessions and command executions have finished.

The lock file intentionally remains after an abnormal process termination.

This is not an operating-system advisory lock. A separate process or administrator can remove the file while its owner is still running.

## Recovery after a crash

1. Stop all Agent processes that might use the affected journal.
2. Confirm that no process is still executing commands associated with that journal.
3. Preserve a backup of the journal and its `.lock` file for investigation.
4. Check that the journal has not been truncated or corrupted.
5. Only after confirming that no active owner remains, remove the stale `.lock` file.
6. Restart the process and verify that the journal opens successfully.

Never delete the journal to resolve a duplicate command ID or lock error.

Never remove an active lock file.

If journal integrity is uncertain, keep execution disabled until the state has been investigated.

## Durability limitations

Successful reservations call `Sync()` on the journal file.

This does not, by itself, guarantee durability of newly created directory entries after sudden power loss.

The implementation has not been validated against filesystem corruption, power-loss simulation or hostile filesystem manipulation.

For production deployments, store the journal on a reliable local filesystem with restricted permissions and operational backups.

## Testing

The registry has tests covering persistence across reopening, duplicate command IDs, malformed records, concurrent ownership, WebSocket reconnection, active executors after disconnect, and abnormal termination of a separate process.

Linux CI runs `go test -race ./...`.

These tests do not constitute a guarantee of crash-safe exactly-once execution.