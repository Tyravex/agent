# Tyravex Agent

Node daemon for Tyravex Panel.

## Requirements

- Go 1.27+
- Tyravex Panel with the Nodes API enabled

## Development

Configure the following environment variables:

- `TYRAVEX_PANEL_URL`
- `TYRAVEX_NODE_ID`
- `TYRAVEX_NODE_TOKEN`
- `TYRAVEX_HEARTBEAT_INTERVAL_MS` (optional, default: 30000)

Run:

```sh
go run ./cmd/tyravex-agent
```
