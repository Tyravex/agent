# Tyravex Agent — Local Docker Test

## Purpose

This test starts a real Minecraft Java server using
the Tyravex Docker runtime.

This command is local only. It is not yet connected
to the Tyravex Panel WebSocket protocol.

## Requirements

- Windows with Docker Desktop in Linux containers mode,
  or Debian with Docker Engine.
- Docker CLI available in PATH.
- Docker daemon running.
- Free TCP port 25565.
- At least 2 GiB RAM available for the container.
- Enough free disk space for the Minecraft image and data.
- Acceptance of the Minecraft EULA.

Minecraft EULA:
https://aka.ms/MinecraftEULA

## Windows

Check Docker:

```powershell
docker version
docker info
```

Build:

```powershell
go build -o tyravex-docker.exe ./cmd/tyravex-docker
```

Install and start the server:

```powershell
.\tyravex-docker.exe minecraft-install -id mc-test -port 25565 -memory 2048 -cpu 100
```

Check status and logs:

```powershell
.\tyravex-docker.exe status mc-test
.\tyravex-docker.exe logs -lines 100 mc-test
```

Once Minecraft finishes starting, connect to:

localhost:25565

Stop, start and restart:

```powershell
.\tyravex-docker.exe stop mc-test
.\tyravex-docker.exe start mc-test
.\tyravex-docker.exe restart mc-test
```

## Debian

Build:

```bash
go build -o tyravex-docker ./cmd/tyravex-docker
```

Install:

```bash
./tyravex-docker minecraft-install -id mc-test -port 25565 -memory 2048 -cpu 100
```

Check:

```bash
./tyravex-docker status mc-test
./tyravex-docker logs -lines 100 mc-test
```

## Docker resources

Container name:

tyravex-mc-test

Persistent volume:

tyravex-data-mc-test

The volume is mounted at /data inside the container.

Stopping or restarting the container does not delete
the server files.

## Limitations

- This is a diagnostic CLI, not the main Agent daemon.
- Installation is not idempotent.
- Running the install command twice with the same ID fails.
- Disk quotas are not enforced.
- Port 25565 is published on host interfaces.
- Protect the server with a firewall.
- Use trusted Docker images.
- Production images should be pinned to a digest.
- The runtime currently uses Docker CLI rather than SDK.
- Panel-to-Agent command execution is not connected yet.
- EULA=TRUE is set by the installer; do not run unless
  you accept the Minecraft EULA.
