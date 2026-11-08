# Tyravex Agent — Project Structure

**Status:** Target architecture
**Implementation:** Incremental
**Go module:** `github.com/Tyravex/agent`

This document records the agreed **target** directory and file structure of Tyravex Agent. It is **not** a listing of files that currently exist. Existing packages will be migrated incrementally, with security boundaries and tests preserved.

The tree below follows the proposed structure. The agreed storage choice is reflected by `internal/state/sqlite.go` instead of `bolt.go`.

## Target repository tree

```text
agent/
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                         # lint + unit + build
│   │   ├── integration.yml                # Docker integration tests
│   │   ├── security.yml                   # govulncheck, gosec, security tests
│   │   └── release.yml                    # GoReleaser, SHA256SUMS, signatures
│   ├── ISSUE_TEMPLATE/
│   └── dependabot.yml
│
├── api/
│   └── protocol/
│       ├── v1/
│       │   ├── envelope.schema.json       # message envelope
│       │   ├── commands.schema.json       # Panel-to-Agent commands
│       │   ├── events.schema.json         # Agent-to-Panel events
│       │   └── errors.schema.json
│       └── README.md                      # protocol versioning rules
│
├── cmd/
│   └── tyravex-agent/
│       ├── main.go                        # Execute() only
│       ├── root.go                        # global flags, configuration loading
│       ├── run.go                         # daemon startup
│       ├── version.go                     # version [--json]
│       ├── dockerinfo.go                  # docker-info
│       ├── configcheck.go                 # config.yml validation
│       └── install.go                     # service installation (systemd/Windows)
│
├── deployments/
│   ├── systemd/
│   │   └── tyravex-agent.service
│   ├── windows/
│   │   └── install-service.ps1
│   └── docker/
│       └── Dockerfile
│
├── docs/
│   ├── README.md
│   ├── architecture.md
│   ├── project-structure.md               # target project tree (this document)
│   ├── protocol.md
│   ├── security.md
│   ├── configuration.md
│   ├── development.md
│   ├── testing.md
│   ├── eggs.md                            # how the Agent interprets eggs
│   └── adr/
│       ├── 0001-outbound-websocket.md
│       ├── 0002-ports-and-adapters.md
│       ├── 0003-local-state-storage.md
│       └── template.md
│
├── internal/
│   ├── app/
│   │   ├── app.go                         # composition root
│   │   ├── lifecycle.go                   # startup/shutdown order, graceful shutdown
│   │   └── wiring.go                      # component construction and injection
│   │
│   ├── buildinfo/
│   │   └── buildinfo.go                   # version, commit, build date (ldflags)
│   │
│   ├── config/
│   │   ├── config.go                      # data structures
│   │   ├── load.go                        # YAML + environment variable overrides
│   │   ├── validate.go                    # minimum 32-character token, API token != node token
│   │   └── defaults.go
│   │
│   ├── logging/
│   │   ├── logger.go                      # slog
│   │   └── redact.go                      # token and secret redaction
│   │
│   ├── auth/
│   │   ├── token.go                       # constant-time comparisons
│   │   ├── middleware.go                  # Bearer authentication for httpapi
│   │   ├── signing.go                     # command signatures (planned)
│   │   └── tls.go                         # mTLS (planned)
│   │
│   ├── transport/
│   │   ├── panelws/
│   │   │   ├── client.go                  # WebSocket connection
│   │   │   ├── heartbeat.go
│   │   │   ├── reconnect.go               # backoff with jitter
│   │   │   ├── outbox.go                  # offline event outbox
│   │   │   └── handshake.go
│   │   ├── protocol/
│   │   │   ├── envelope.go                # ID, type, version, payload
│   │   │   ├── commands.go
│   │   │   ├── events.go
│   │   │   ├── errors.go                  # error codes
│   │   │   └── codec.go                   # JSON + validation
│   │   └── httpapi/
│   │       ├── server.go                  # loopback-only binding
│   │       ├── routes.go
│   │       ├── health.go
│   │       ├── status.go
│   │       └── docker.go
│   │
│   ├── command/
│   │   ├── dispatcher.go                  # routing, timeouts, acknowledgments/results
│   │   ├── registry.go
│   │   ├── idempotency.go                 # deduplication by command ID
│   │   ├── result.go
│   │   └── handlers/
│   │       ├── server_create.go
│   │       ├── server_start.go
│   │       ├── server_stop.go
│   │       ├── server_restart.go
│   │       ├── server_kill.go
│   │       ├── server_reinstall.go
│   │       ├── server_delete.go
│   │       ├── server_suspend.go
│   │       ├── console_send.go
│   │       ├── files_list.go
│   │       ├── files_read.go
│   │       ├── files_write.go
│   │       ├── files_delete.go
│   │       ├── files_archive.go
│   │       ├── backup_create.go
│   │       ├── backup_restore.go
│   │       ├── schedule_sync.go
│   │       └── node_info.go
│   │
│   ├── server/
│   │   ├── server.go                      # entity
│   │   ├── state.go                       # state machine
│   │   ├── manager.go                     # registry and lifecycle operations
│   │   ├── limits.go                      # CPU/RAM/PID/disk
│   │   ├── events.go
│   │   ├── install.go                     # installation pipeline
│   │   ├── crashdetect.go                 # crash detection, auto-restart with backoff
│   │   └── errors.go
│   │
│   ├── runtime/
│   │   ├── runtime.go                     # Runtime interface (port)
│   │   ├── types.go                       # ContainerSpec, Stats, LogLine...
│   │   ├── errors.go
│   │   ├── docker/
│   │   │   ├── runtime.go
│   │   │   ├── client.go                  # Docker SDK wrapper
│   │   │   ├── create.go
│   │   │   ├── lifecycle.go
│   │   │   ├── attach.go
│   │   │   ├── logs.go
│   │   │   ├── stats.go
│   │   │   ├── images.go                  # image pull and cache
│   │   │   ├── network.go
│   │   │   ├── volumes.go
│   │   │   ├── hardening.go               # cap-drop, no-new-privileges, PID limits
│   │   │   ├── reconcile.go               # state reconciliation after restart
│   │   │   └── labels.go                  # tyravex.* labels
│   │   └── fake/
│   │       └── runtime.go                 # test implementation
│   │
│   ├── egg/
│   │   ├── egg.go                         # model
│   │   ├── parser.go
│   │   ├── validate.go
│   │   ├── variables.go                   # variables and substitution
│   │   ├── installer.go                   # installation script inside container
│   │   ├── startup.go                     # startup command construction
│   │   └── importer.go                    # Pterodactyl/Pelican egg import
│   │
│   ├── console/
│   │   ├── console.go
│   │   ├── buffer.go                      # recent-line buffer
│   │   ├── stream.go                      # fan-out to multiple subscribers
│   │   ├── input.go                       # command input
│   │   └── readydetect.go                 # server readiness detection from logs
│   │
│   ├── stats/
│   │   ├── collector.go
│   │   ├── container.go
│   │   ├── host.go                        # node CPU, RAM, disk, network
│   │   └── publisher.go
│   │
│   ├── filesystem/
│   │   ├── sandbox.go                     # path traversal, symlinks
│   │   ├── ops.go                         # list, read, write, delete, rename, chmod
│   │   ├── archive.go                     # zip/tar.gz, zip-slip protection
│   │   ├── quota.go
│   │   └── upload.go
│   │
│   ├── sftp/
│   │   ├── server.go
│   │   ├── handler.go                     # uses filesystem/
│   │   ├── auth.go
│   │   └── hostkey.go
│   │
│   ├── backup/
│   │   ├── backup.go
│   │   ├── create.go
│   │   ├── restore.go
│   │   ├── retention.go
│   │   ├── checksum.go
│   │   └── target/
│   │       ├── target.go                  # target interface
│   │       ├── local.go
│   │       └── s3.go
│   │
│   ├── scheduler/
│   │   ├── scheduler.go
│   │   ├── cron.go
│   │   ├── task.go
│   │   ├── chain.go                       # task chains
│   │   └── actions.go                     # restart, command, backup
│   │
│   ├── network/
│   │   ├── ports.go                       # port allocation
│   │   ├── isolation.go
│   │   └── firewall.go
│   │
│   ├── state/
│   │   ├── store.go                       # interface
│   │   ├── sqlite.go                      # SQLite in WAL mode
│   │   ├── servers.go
│   │   ├── outbox.go
│   │   └── migrations.go
│   │
│   ├── system/
│   │   ├── info.go
│   │   ├── info_linux.go
│   │   ├── info_windows.go
│   │   ├── paths.go
│   │   ├── paths_linux.go
│   │   ├── paths_windows.go
│   │   └── service.go                     # service manager integration
│   │
│   └── updater/
│       ├── updater.go
│       ├── release.go                     # download from GitHub Releases
│       ├── verify.go                      # SHA256SUMS / signatures
│       └── swap.go                        # binary replacement and restart
│
├── scripts/
│   ├── build.ps1
│   ├── build.sh
│   ├── install.sh                         # one-line installer
│   ├── gen-token.sh
│   └── lint.sh
│
├── test/
│   ├── integration/                       # //go:build integration
│   │   ├── main_test.go                   # TestMain: clean up containers labeled tyravex.test
│   │   ├── runtime/
│   │   │   ├── lifecycle_test.go
│   │   │   ├── limits_test.go
│   │   │   └── network_test.go
│   │   ├── console/
│   │   │   ├── attach_test.go
│   │   │   └── logs_test.go
│   │   ├── filesystem/
│   │   │   └── ops_test.go
│   │   ├── backup/
│   │   │   └── restore_test.go
│   │   └── recovery/
│   │       └── reconcile_test.go
│   │
│   ├── e2e/                               # //go:build e2e
│   │   ├── lifecycle/
│   │   │   └── server_lifecycle_test.go
│   │   ├── reconnect/
│   │   │   └── reconnect_test.go
│   │   ├── updates/
│   │   │   └── update_test.go
│   │   └── helpers/
│   │       ├── agentproc.go               # launching the Agent binary
│   │       └── scenario.go
│   │
│   ├── contract/
│   │   ├── protocol/
│   │   │   ├── commands_test.go
│   │   │   └── events_test.go
│   │   └── testdata/
│   │       ├── commands/
│   │       └── events/
│   │
│   ├── security/
│   │   ├── auth/
│   │   │   ├── token_test.go
│   │   │   └── replay_test.go
│   │   ├── sandbox/
│   │   │   ├── traversal_test.go
│   │   │   ├── symlink_test.go
│   │   │   └── zipslip_test.go
│   │   └── hardening/
│   │       └── container_flags_test.go
│   │
│   ├── performance/
│   │   ├── console/
│   │   │   └── throughput_bench_test.go
│   │   └── stats/
│   │       └── collector_bench_test.go
│   │
│   ├── fixtures/
│   │   ├── eggs/
│   │   │   ├── minecraft.json
│   │   │   ├── discord-bot-node.json
│   │   │   ├── telegram-bot-python.json
│   │   │   └── teamspeak3.json
│   │   └── configs/
│   │       ├── valid.yml
│   │       └── invalid-short-token.yml
│   │
│   └── testutil/
│       ├── docker.go                      # t.Skip when Docker is unavailable
│       ├── tempdir.go
│       ├── assert.go
│       └── fakepanel/
│           ├── server.go                  # fake Panel over WebSocket
│           └── recorder.go                # recording received messages
│
├── .gitattributes
├── .gitignore
├── .golangci.yml
├── .goreleaser.yaml
├── CHANGELOG.md
├── CONTRIBUTING.md
├── LICENSE
├── Makefile                               # or Taskfile.yml
├── README.md
├── SECURITY.md
├── VERSION
├── config.example.yml
├── go.mod
└── go.sum
```

## Migration rules

1. **Do not create placeholder packages** solely to reproduce the target tree.
2. Preserve existing production code, tests, and Git history during migration.
3. Keep package moves separate from behavioral changes, especially changes to ownership or runtime authorization.
4. Do not enable ownership-bound production lifecycle execution as a side effect of restructuring.
5. Prefer a small, reviewable change with passing tests over a broad rewrite.
6. Treat the tree as a design reference; update this document when an explicit architectural decision changes it.

## Related documentation

- [Architecture](architecture.md) — design principles, communication, authorization, and migration policy.
- `docs/adr/` — planned architectural decision records.
- `api/protocol/README.md` — planned protocol versioning policy.
