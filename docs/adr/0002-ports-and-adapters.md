# ADR-0002: Ports and Adapters for Runtime and Infrastructure

- **Status:** Accepted (target architecture)
- **Date:** 2026-11-08
- **Scope:** Application composition and dependency boundaries

## Context

Tyravex Agent coordinates transport, authorization, server lifecycle, Docker, filesystem operations, and local state. Directly coupling domain behavior to the Docker SDK, WebSocket implementation, or SQLite would make testing and future migration harder. The repository already contains working packages and security guards that must remain intact during restructuring.

## Decision

Adopt ports and adapters incrementally. Business and lifecycle policy depend on explicit interfaces rather than concrete infrastructure clients. The target composition root is `internal/app/`, responsible for wiring dependencies, startup order, cancellation, and graceful shutdown.

The target runtime port lives in `internal/runtime/`, with a Docker adapter under `internal/runtime/docker/` and a deterministic fake under `internal/runtime/fake/`. Other infrastructure boundaries, including transport and storage, follow the same principle where useful. Interfaces should be small, operation-oriented, and introduced when concrete behavior or tests need them.

The existing `internal/agent`, `internal/commands`, `internal/docker`, and `internal/transport` packages remain in place until separately scoped migrations are justified. Do not introduce empty packages merely to match the target directory tree.

## Security boundaries

- The runtime adapter executes operations but never grants ownership authority.
- Lifecycle authorization must be checked at the application/command boundary and remain bound to the authorized node, session, generation, and runtime endpoint.
- No fallback from a guarded executor to an unguarded executor is allowed.
- Docker SDK and Docker CLI operations must resolve to the same explicitly validated local endpoint when they participate in an ownership-bound workflow.
- Remote Docker endpoints must not be selected implicitly from ambient environment configuration.
- Fake adapters must not weaken production validation; tests must include negative authorization cases.

## Consequences

**Benefits:** deterministic unit tests, clearer responsibilities, isolated infrastructure dependencies, and easier incremental evolution.

**Costs:** interface design and dependency wiring add complexity; premature abstraction and broad package moves could create regressions.

## Migration and verification

Introduce a port together with a real use case and tests. Keep refactoring commits separate from behavior and security changes. Preserve existing protocol behavior and ownership regression tests. Production `Runner.Run()` must continue to use its current connection path until a distinct security review approves lifecycle activation.

## Related documents

- `docs/ARCHITECTURE.md`
- `docs/project-structure.md`
- `docs/adr/0001-outbound-websocket.md`
