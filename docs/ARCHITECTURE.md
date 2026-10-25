# Tyravex Agent — architecture

## Scope and status

Tyravex Agent is a Go node daemon designed for Windows with Docker Desktop (Linux containers) and Linux/Debian with Docker Engine. This document distinguishes **implemented** code from **planned** components. Do not treat the planned Docker executor as available yet.

## Existing packages

- `cmd/tyravex-agent/`: CLI entry point, daemon startup and graceful shutdown.
- `internal/config/`: strict YAML configuration and API/network validation.
- `internal/agent/`: node runner and Panel connectivity orchestration.
- `internal/transport/`: authenticated WebSocket, handshake, heartbeat and strict install-command decoding. **Currently rejects execution** of `service.install`.
- `internal/docker/`: Docker client connection and read-only diagnostics.
- `internal/api/`: loopback-only HTTP health/status/Docker diagnostics.
- `internal/version/`: version and build metadata.
- `internal/services/`: engine-independent service specification and validation (added in this batch).

## Module boundaries

1. Transport owns protocol parsing, session lifecycle and acknowledgements, not Docker operations.
2. Commands (planned) route validated operations and coordinate idempotency.
3. Services owns the lifecycle and orchestration of managed services; no WebSocket or HTTP dependencies.
4. Docker owns Docker API operations and OS-specific host connections; no Panel dependencies.
5. Configuration and version modules remain independent.
6. API is a local diagnostics/control surface and must not expose unauthenticated Docker operations.

Target dependency direction: `transport -> commands -> services -> docker`; `api -> services` where appropriate. Avoid cycles.

## Tests

- Go unit tests live next to code in `*_test.go` files.
- `tests/integration/` (planned) is reserved for Docker Engine tests; use `//go:build integration` and isolated resources.
- `tests/e2e/` (planned) is reserved for Panel-Agent-Docker tests; use `//go:build e2e`.
- Never run destructive integration tests against production Docker contexts.
- CI currently runs formatting, vet, unit tests and Linux race tests on Windows/Linux.

## Docker executor requirements (not yet implemented)

- Stable service identity and explicit ownership labels; never adopt unrelated containers.
- No privileged containers, Docker socket mounts, host network, or arbitrary host bind mounts.
- Controlled image pull, container creation and startup; explicit port allocations and persistent volumes.
- Resource limits and filesystem quota semantics must be validated per host; `diskMb` alone does not enforce disk usage.
- Durable command handling: never acknowledge acceptance before execution is safely scheduled; never report success before Docker confirms it.
- On disconnect/restart, reconcile actual Docker state before attempting any retry; do not duplicate installs.
- Separate protocol acceptance from execution result, with tests for both.

## Developer checks

```sh
gofmt -w internal/services/spec.go internal/services/spec_test.go
go vet ./...
go test ./...
go build ./cmd/tyravex-agent
```

The repository `go.mod` controls the required Go toolchain. Run checks on Windows and Debian/Linux before declaring cross-platform support verified.
