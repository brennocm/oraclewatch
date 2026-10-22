# FW-CHAIN-001: Chain head stalled / RPC lagging

- **Maximum severity:** critical
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

Per endpoint, `now - head.timestamp > max_head_lag` marks it lagging and excludes it from the cycle (warning). All endpoints lagging is critical and makes feed freshness checks Unknown.

## Why it matters

Separates "the feed is old" from "my view of the chain is old" and stops an RPC stuck far behind from dragging the pinned block back.

## Related incident

Arbitrum sequencer outage, Dec 2023.

## Parameters

`max_head_lag` per network (5m L1, 2m L2).

## Known false positives and tuning

Host clock skew makes heads look old: run NTP.
