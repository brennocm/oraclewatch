# 7. Alert and monitoring state lives in memory (v1)

Date: 2026-10-22
Status: accepted

## Context

The alert manager tracks, per (check, network, feed), whether a condition is pending,
firing or resolving, when it started, and when it was last sent. The engine keeps the
previous reading, metadata and bounds of each feed, which `FW-INVALID-001` (round going
backwards), `FW-AGG-001` and `FW-JUMP-001` compare against.

## Decision

v1 keeps all of this in memory. Nothing is written to disk.

## Alternatives

- **SQLite or a JSON file on a volume.** Survives restarts, but adds a writable volume to
  a container that is otherwise `read_only`, a schema to migrate, and corruption cases to
  handle.
- **Prometheus/Alertmanager for state.** Possible for the alerting part, but would make the
  Discord/Telegram path depend on another service.

## Consequences

After a restart:

- Firing alerts are forgotten. A condition that still holds fires again after `for:`
  (default 2 minutes): one duplicate message, never a missed one.
- No "resolved" is sent for conditions that cleared while the process was down.
- `FW-AGG-001` and `FW-META-001` version changes that happened while it was down are not
  reported (the first read becomes the baseline). `FW-META-001`'s decimals and description
  checks compare with the configuration, so they are unaffected.
- `FW-INVALID-001`'s backwards-round and `FW-JUMP-001` have no previous round for the
  first cycle.

Restarts are visible from outside: `FW-SELF-001`'s dead man's switch stops being pinged
while the process is down. Persistence is listed as future work.
