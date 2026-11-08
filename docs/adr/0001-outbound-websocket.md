# ADR-0001: Outbound WebSocket Control Channel

- **Status:** Accepted (target architecture)
- **Date:** 2026-11-08
- **Scope:** Tyravex Agent–Panel communication

## Context

The Agent must manage nodes that may be behind NAT or restrictive firewalls. A public inbound Agent listener would increase deployment complexity and attack surface. The existing implementation already uses an outbound WebSocket connection; future console and file-transfer features must not undermine its security boundary.

## Decision

The Agent initiates and maintains a TLS-protected outbound WebSocket (`wss://`) control connection to the Panel. The control channel carries node handshake and identity, heartbeats, ownership synchronization, commands, acknowledgements, results, and operational events.

No publicly reachable inbound Agent port is required for ordinary operation. Console streaming and bulk file transfers may use separate, explicitly authenticated outbound sessions or a Panel-mediated relay. Short-lived signed URLs, if used, are scoped access tickets, not permission to bypass node, session, resource, or ownership authorization, and do not imply direct public access to the Agent.

An optional local HTTP diagnostic API is separate from the control channel, binds to loopback by default, and must enforce authentication and operation-specific authorization.

## Security and reliability requirements

- Validate TLS certificates and the authenticated Panel identity; do not disable verification in production.
- Bind commands and ownership to the authenticated node and current connection/session context.
- Apply message size limits, strict decoding, timeouts, heartbeat checks, and bounded reconnect backoff with jitter.
- Use bounded queues and backpressure so bulk transfers cannot starve control messages.
- Treat reconnect as a new authorization boundary: do not resurrect ownership solely from local state.
- Reject state-changing operations when authorization or session binding is missing or stale.
- Preserve compatibility with the existing wire protocol until a separately tested protocol migration.

## Consequences

**Benefits:** outbound-first deployment, fewer public endpoints, centralized authentication, and clear separation between control and bulk data.

**Costs:** reconnect handling, session renewal, offline event delivery, and relay/stream authorization require explicit designs and tests.

## Implementation and verification

This ADR is an architectural decision, not permission to enable production lifecycle execution. Verify handshake and identity checks, reconnect behavior, ownership invalidation, TLS/WebSocket interoperability, and rejection of unauthorized commands in tests before changing production wiring.

## Related documents

- `docs/ARCHITECTURE.md`
- `docs/project-structure.md`
- `docs/adr/0003-local-state-storage.md`
