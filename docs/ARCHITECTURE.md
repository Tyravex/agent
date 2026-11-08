# Tyravex Agent — Architecture

**Status:** Target architecture
**Implementation:** Incremental
**Go module:** `github.com/Tyravex/agent`

> This document describes the agreed target architecture, not a claim that every component already exists. Existing code and security controls remain in place during incremental migration.

## 1. Purpose

Tyravex Agent manages game and application servers on a node. It communicates with Tyravex Panel, executes authorized operations through container runtimes, publishes events, and maintains local operational state.

**Security-sensitive operations must fail closed.**

## 2. Core principles

1. **Outbound-first:** the Agent initiates the control connection to the Panel.
2. **No public inbound port by default:** basic management must work behind NAT and firewalls.
3. **Versioned JSON protocol:** JSON and JSON Schema, with strict runtime validation in Go and TypeScript.
4. **Ports and adapters:** infrastructure details are isolated behind explicit interfaces.
5. **Docker behind a runtime interface:** fake implementations support deterministic tests.
6. **SQLite WAL for local state:** persistent outbox, idempotency records, and server metadata.
7. **Explicit lifecycle authorization:** current ownership is mandatory for state-changing server operations.
8. **No authorization resurrection:** persisted data must not restore ownership after a restart.
9. **Least privilege:** local endpoint restrictions, filesystem isolation, secret redaction, and container hardening.
10. **Incremental migration:** no large package moves combined with behavior changes.

## 3. Communication architecture

```text
Tyravex Panel
    |
    |  outbound connection initiated by Agent
    |  TLS / WSS: handshake, commands, events, heartbeat, ownership
    v
Tyravex Agent
    |
    +-- Authorization / ownership
    +-- Command dispatcher / idempotency
    +-- Runtime port --> Docker adapter
    +-- SQLite WAL --> outbox and durable operational state
```

The Agent establishes and maintains an outbound TLS-protected WebSocket control connection. The control channel carries:

- Node identity, handshake, and heartbeats.
- Ownership synchronization and session binding.
- Commands, acknowledgements, results, and operational events.

Console streams and file transfers may use separate authenticated WSS/HTTPS sessions or a relay. **Signed, short-lived URLs are access tickets, not a requirement to expose a public Agent endpoint.** Bulk transfer must not starve the control channel.

An optional local HTTP diagnostic API, if implemented, must bind to loopback by default, require authentication and explicit authorization, and must not bypass the ownership guard.

## 4. Protocol

The target source of truth for protocol schemas is `api/protocol/v1/`:

- `envelope.schema.json`
- `commands.schema.json`
- `events.schema.json`
- `errors.schema.json`

`api/protocol/README.md` defines versioning and compatibility policy. The existing JSON envelope and message types remain in force until an explicitly tested migration. Schema validation is complementary to strict runtime checks, including message size limits, duplicate-key rejection, identity matching, and session validation.

Protocol changes must be coordinated with the Panel and covered by contract tests. The Go and TypeScript implementations must agree on the meaning of every field.

## 5. Target application layers

| Layer              | Planned location                                       | Responsibility                                |
| ------------------ | ------------------------------------------------------ | --------------------------------------------- |
| Composition root   | `internal/app/`                                        | Dependency wiring, startup, shutdown          |
| Transport          | `internal/transport/`                                  | WebSocket, protocol codec, optional local API |
| Authentication     | `internal/auth/`                                       | Enrollment, credentials, signing, TLS         |
| Commands           | `internal/command/`                                    | Dispatch, deduplication, result handling      |
| Server domain      | `internal/server/`                                     | Server state and lifecycle policy             |
| Runtime port       | `internal/runtime/`                                    | Runtime-independent operations                |
| Docker adapter     | `internal/runtime/docker/`                             | Docker SDK and container operations           |
| Persistent state   | `internal/state/`                                      | SQLite, migrations, outbox, idempotency       |
| Filesystem         | `internal/filesystem/`                                 | Sandboxed file operations                     |
| Supporting modules | `internal/egg/`, `console/`, `stats/`, `backup/`, etc. | Domain-specific features                      |

These paths are **targets**. Current packages such as `internal/agent`, `internal/commands`, `internal/docker`, and `internal/transport` must not be moved solely to make the tree look complete.

## 6. Command authorization pipeline

A state-changing command must pass the relevant checks in order:

1. Transport and protocol validation.
2. Node identity and authenticated session validation.
3. Signature, freshness, and replay validation when implemented.
4. Command idempotency and duplicate handling.
5. **Current ownership authorization** for lifecycle operations.
6. Resource-specific policy and input validation.
7. Runtime execution.
8. Durable result/event handling and delivery.

A valid command signature is **not** a substitute for ownership. There must be no fallback to an unguarded executor if authorization fails.

## 7. Ownership security boundary

Ownership is a live authorization decision, not a cached preference. Lifecycle execution must be rejected when ownership is missing, stale, revoked, bound to another node/session/generation, or associated with an unauthorized runtime endpoint.

The Runner must also reject ownership activation after relevant configuration changes. A manually injected runtime or forged local Docker endpoint cannot be treated as proof that the trusted local factory was used.

**Persisted SQLite records may restore history, but never restore the right to start, stop, or restart containers.** After restart or reconnection, current authenticated ownership must be established again.

At the time of this document, production `Runner.Run()` uses the ordinary transport connection and **does not enable ownership-bound lifecycle execution**. The guarded local ownership connection entry point exists for controlled use and testing; it must not be wired into production without a separate security review.

## 8. Runtime abstraction

The target `internal/runtime/` package defines runtime-independent contracts. Docker is an adapter, and `internal/runtime/fake/` provides test implementations.

The runtime performs operations; it does not grant authorization. The authorized local Docker endpoint must be explicit and consistent between the Docker SDK and CLI. Remote Docker endpoints must not be activated implicitly through environment variables or configuration drift.

## 9. Local state: SQLite WAL

SQLite in WAL mode is the selected target local store. Planned persisted data includes:

- Server metadata and reconciliation metadata.
- Event outbox for temporary Panel disconnections.
- Command IDs, execution status, and results for durable idempotency.
- Schema version and migrations.

WAL does not make Docker operations and SQLite transactions atomic. Each state-changing workflow needs documented crash-recovery and reconciliation semantics. The outbox must have bounded retention and delivery/retry rules. Secrets must be protected and must not be logged.

## 10. Authentication and replay protection

The target design includes:

- Short-lived, single-use enrollment tokens.
- Node-bound credentials with explicit rotation and revocation.
- Authenticated TLS transport.
- Authenticated state-changing commands (for example, HMAC with a versioned canonical representation).
- Command IDs, timestamps, session/generation binding, and persistent replay/idempotency checks.

A timestamp alone does not prevent replay within its validity window. The signature must cover all security-relevant fields, including the command identity, node identity, operation, target resource, payload, and session context. The exact canonicalization and key-rotation protocol must be specified before implementation.

## 11. Isolation and hardening

Filesystem operations must enforce per-server boundaries and prevent path traversal, symlink escapes, and archive extraction escapes. Container creation must apply explicit resource limits and hardening policies. Sensitive credentials and tokens must be redacted from logs and error output.

## 12. Testing strategy

- Unit tests and fake runtimes for deterministic behavior.
- Protocol contract tests against shared JSON schemas.
- Local TLS/WebSocket integration tests using trusted test certificates.
- Docker integration tests with isolated resources and cleanup.
- End-to-end tests for lifecycle, reconnect, and updates.
- Security regression tests for replay, ownership, filesystem isolation, and container hardening.
- Performance benchmarks for streaming and telemetry.

Production credentials and a live Panel must not be required by the normal test suite. Tests requiring Docker should be explicitly separated from ordinary unit tests.

## 13. Build, release, and operations

The target delivery pipeline includes CI, Docker integration, security analysis, and signed releases with checksums. Build metadata should record version, commit, and build date. Installation must support Windows services and Linux systemd without changing security defaults.

## 14. Migration policy

1. Document the target structure in `docs/project-structure.md`.
2. Preserve existing packages and tests until a scoped migration is justified.
3. Introduce a target directory when an actual implementation or test needs it; avoid empty scaffolding.
4. Keep structural refactors separate from security behavior changes.
5. Run unit, integration, vet, and build checks appropriate to each change.
6. Never activate production lifecycle execution merely as part of reorganizing files.
7. Require an explicit review before changing the ownership authorization boundary.

## 15. Architecture decisions

Planned ADRs under `docs/adr/`:

- `0001-outbound-websocket.md`
- `0002-ports-and-adapters.md`
- `0003-local-state-storage.md`
- `template.md`

These ADRs are planned documents; their presence in the target tree does not imply they have already been created.
