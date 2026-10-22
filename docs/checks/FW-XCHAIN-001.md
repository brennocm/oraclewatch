# FW-XCHAIN-001: Cross-chain divergence

- **Maximum severity:** warning
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

For the same pair on networks A and B (each read at its own pinned block): `|a - b| / min(a, b) > threshold_a + threshold_b + margin`. Stale or unread entries are skipped.

## Why it matters

Each feed may lag by up to its own threshold, so two healthy feeds can differ by their sum; beyond that, one network's feed is stuck or wrong.

## Related incident

—

## Parameters

`cross_chain.margin` (1%).

## Known false positives and tuning

SVR feeds on Base may add a delay; raise the margin if needed.
