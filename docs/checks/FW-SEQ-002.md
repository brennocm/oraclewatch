# FW-SEQ-002: L2 sequencer grace period

- **Maximum severity:** warning
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

Sequencer up and `blockTime - startedAt <= grace_period`, the same comparison as the official consumer example.

## Why it matters

Consumers following the docs reject prices during this window; the operator should know the protocol is effectively paused.

## Related incident

Same as FW-SEQ-001.

## Parameters

`sequencer.grace_period` (1h, the example's GRACE_PERIOD_TIME).

## Known false positives and tuning

Match your protocol's own grace period.
