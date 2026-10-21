
# Tyravex Agent

Standalone node daemon for Tyravex Panel, written in Go.

## Features

- Authenticated heartbeat to Tyravex Panel
- Local HTTP API with separate authentication token
- Docker Engine diagnostics
- YAML configuration
- Semantic versioning and build metadata
- Graceful HTTP server shutdown
- Windows and Linux support

## Requirements

- Tyravex Panel with Nodes API
- Docker Engine or Docker Desktop for Docker diagnostics
- Go version specified in `go.mod` for development

## Configuration

Copy `config.example.yml` to `config.yml`.

Configure:

| Field | Description |
|---|---|
| `panel.url` | Tyravex Panel API URL |
| `node.id` | Node identifier |
| `node.token` | Token for Agent-to-Panel heartbeat |
| `heartbeat.interval` | Heartbeat interval, default `30s` |
| `docker.enabled` | Enable Docker diagnostics |
| `docker.host` | Docker Engine connection address |
| `api.listen` | Local HTTP listen address |
| `api.token` | Separate token for HTTP API |

Example API configuration:

```yaml
api:
  listen: "127.0.0.1:8080"
  token: "REPLACE_WITH_A_SECURE_RANDOM_TOKEN"
```

The API token must contain at least 32 characters and must
differ from `node.token`.

Generate a cryptographically secure random token.
Do not commit `config.yml`.

## HTTP API

The HTTP API is bound to a loopback IP address only.

| Method | Endpoint | Authentication |
|---|---|---|
| GET | `/health` | None |
| GET | `/api/v1/status` | Bearer API token |
| GET | `/api/v1/docker` | Bearer API token |

### Health check

```sh
curl http://127.0.0.1:8080/health
```

Expected response:

```json
{"status":"ok"}
```

### Agent status

```sh
curl \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  http://127.0.0.1:8080/api/v1/status
```

Returns node ID, status, uptime, version metadata and
Docker integration status.

### Docker diagnostics

```sh
curl \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  http://127.0.0.1:8080/api/v1/docker
```

Returns Docker Engine version and container information.

When Docker integration is disabled or unavailable,
the endpoint returns HTTP 503.

Never expose the HTTP API directly to the public internet.
Remote Panel-to-Agent access requires a secure transport
mechanism, which is not yet implemented.

## Commands

Start the agent:

```sh
go run ./cmd/tyravex-agent
```

Show version:

```sh
go run ./cmd/tyravex-agent version
```

Show version as JSON:

```sh
go run ./cmd/tyravex-agent version --json
```

Inspect Docker Engine:

```sh
go run ./cmd/tyravex-agent docker-info
```

## Development

Run checks:

```sh
go fmt ./...
go vet ./...
go test ./...
```

Build on Windows:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ".\scripts\build.ps1"
```

Only run trusted scripts.

## Releases

Release archives are published for:

- Windows AMD64
- Linux AMD64
- Linux ARM64

Each release includes `SHA256SUMS` for download verification.

See [GitHub Releases](https://github.com/Tyravex/agent/releases).

## Security

- Keep `config.yml` private.
- Use different tokens for heartbeat and HTTP API.
- Generate tokens using a cryptographically secure random generator.
- Do not expose the Docker Engine API over unauthenticated TCP.
- Prefer a local Docker socket or Windows named pipe.
- Protect access to Docker sockets and Agent configuration files.
- The local HTTP API does not provide TLS.
- Do not expose the local API directly over a public network.
```
