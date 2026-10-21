# Tyravex Agent

Standalone node daemon for Tyravex Panel, written in Go.

## Features

- Authenticated heartbeat to Tyravex Panel
- YAML configuration
- Docker Engine diagnostics
- Semantic versioning and build metadata
- Windows and Linux support

## Requirements

- Tyravex Panel with Nodes API
- Docker Engine or Docker Desktop for Docker diagnostics
- Go version specified in `go.mod` for development

## Configuration

Copy `config.example.yml` to `config.yml`.

Edit the following fields:

- `panel.url` — Tyravex Panel API URL
- `node.id` — node identifier
- `node.token` — node authentication token
- `heartbeat.interval` — heartbeat interval, e.g. `30s`
- `docker.enabled` — enable Docker integration
- `docker.host` — Docker Engine connection address

Keep `config.yml` private. Never commit node tokens.

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

Build on Windows using PowerShell:

```powershell
.\scripts\build.ps1
```

## Releases

Release archives are published for:

- Windows AMD64
- Linux AMD64
- Linux ARM64

Each release includes a `SHA256SUMS` file for verifying downloads.

See [GitHub Releases](https://github.com/Tyravex/agent/releases).

## Security

Do not expose the Docker Engine API over unauthenticated TCP.

Prefer a local Docker socket or Windows named pipe.
Protect access to the Docker socket and the agent configuration file.