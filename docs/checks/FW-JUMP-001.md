# FW-JUMP-001: Abrupt round-to-round change

- **Maximum severity:** warning
- **Status:** experimental (off by default)
- **Implementation:** `internal/checks/`

## What it checks

`|answer_now - answer_prev| / answer_prev > max_jump`, comparing with the round seen in the previous cycle.

## Why it matters

Catches implausible values on feeds with no market reference, such as exchange-rate feeds.

## Related incident

Moonwell wrsETH/ETH, Nov 2025 (incidents.md §2).

## Parameters

`jump.enabled` (false), `jump.max_jump` (20%).

## Known false positives and tuning

Volatile assets can move 20% between two cycles in a crash; raise `max_jump` for them. Several rounds between cycles are compared as one step.
