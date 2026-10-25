# Tyravex Docker runtime — MVP

This batch adds `internal/docker/runtime.go` and its unit tests. The existing Docker SDK diagnostic client is unchanged.

## Current scope

- Docker CLI adapter for Windows Docker Desktop (Linux containers) and Debian Docker Engine.
- Image pull, named persistent `/data` volume, container creation and start.
- CPU/memory limits, TCP port mapping, security flags and Tyravex ownership labels.
- Start, stop, restart, status and bounded logs for labeled containers.
- No WebSocket command execution or Panel integration yet.

## Limitations before production

- Requires the `docker` executable in PATH and a working Docker daemon.
- No disk quota enforcement. Docker named volumes persist but are not capped by `diskMb`.
- TCP-only port mapping; no UDP or multiple allocations.
- No idempotent install/reconciliation yet: existing container name makes install fail safely.
- Image and environment values must come from trusted administrative configuration.
- Environment values are passed as process arguments to Docker CLI and may be observable locally; do not use this adapter for secrets.
- Docker access is highly privileged. Do not expose it through an unauthenticated API.
- This package is not yet wired into the Agent command transport.

## Tests

`go test ./internal/docker/...`

The unit tests do not require Docker. A real integration test and Panel protocol integration are planned separately.
