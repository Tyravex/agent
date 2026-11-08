# ADR-0003: SQLite WAL for Local Operational State

- **Status:** Accepted (target architecture)
- **Date:** 2026-11-08
- **Scope:** Local persistence, idempotency, and offline delivery

## Context

The Agent needs durable operational metadata, command deduplication, and event delivery across transient Panel disconnections and process restarts. In-memory state alone cannot provide reliable replay handling or an offline outbox. However, durable records must not be mistaken for current permission to control containers.

## Decision

Use SQLite with write-ahead logging (WAL) as the target local store. The target package is `internal/state/`, with `sqlite.go`, migrations, server metadata, command idempotency records, reconciliation metadata, and an event outbox.

Persist command identifiers, relevant node/resource scope, execution states, and outcomes so duplicate delivery cannot silently repeat a state-changing operation. Persist outbound events with bounded retention and explicit retry/acknowledgement rules. Database schema changes use versioned migrations.

**Never persist ownership as automatically restorable execution authority.** After restart or reconnect, lifecycle execution requires fresh authenticated ownership synchronization and validation against the current node, session/generation, and authorized runtime endpoint. Persisted history may inform reconciliation but cannot authorize it.

## Consistency and recovery requirements

SQLite transactions and Docker API operations are not atomic with each other. Every state-changing workflow must define durable intent, execution status, crash recovery, and reconciliation against actual Docker state before retrying. A command left in an uncertain state must not be blindly executed again. Command-result publication must not claim success before the runtime confirms the operation.

Use a bounded, single-node database lifecycle, appropriate busy timeouts, integrity checks, backups, and explicit WAL checkpoint/retention policy. Protect local database permissions and avoid storing plaintext secrets unless a separate threat model and protection mechanism justify it.

## Security and reliability requirements

- Define idempotency scope and retention before implementing command execution.
- Reject replayed or mismatched commands; timestamps alone are insufficient replay protection.
- Ensure the outbox cannot grow without bounds or block safety-critical control operations.
- Handle corruption, disk-full conditions, migration failures, and partial runtime operations fail closed.
- Reconcile with Docker using explicit identity and ownership labels; never adopt unrelated containers.
- Keep persisted state distinct from live authorization and credential validity.

## Consequences

**Benefits:** durable idempotency, offline event delivery, transactional metadata updates, and an established migration model.

**Costs:** schema management, disk operations, checkpointing, recovery logic, and cross-system consistency tests.

## Implementation and verification

This ADR selects the storage technology; it does not mean SQLite is already integrated. Implement storage behind a narrow interface and test restart/reconnect, duplicate commands, crash windows, disk failures, migration failures, and ownership non-restoration before relying on persisted execution state.

## Related documents

- `docs/ARCHITECTURE.md`
- `docs/project-structure.md`
- `docs/adr/0001-outbound-websocket.md`
