# FW-RPC-002: RPC fork view

- **Maximum severity:** warning
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

An endpoint returns a different hash for the pinned block than the majority (ties: configuration order). It is excluded from the cycle.

## Why it matters

Short reorgs or an endpoint following another fork; comparing data from different blocks would be meaningless.

## Related incident

—

## Parameters

`confirmations` per network.

## Known false positives and tuning

Raise `confirmations` if this fires on shallow reorgs.
