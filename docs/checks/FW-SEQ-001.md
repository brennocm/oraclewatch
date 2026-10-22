# FW-SEQ-001: L2 sequencer down

- **Maximum severity:** critical
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

Sequencer uptime feed `answer == 1` (down). `answer` not in {0, 1} or `startedAt == 0` is an invalid sequencer round, also critical.

## Why it matters

While the sequencer is down, prices on the L2 cannot update; on recovery queued transactions can liquidate users at stale prices.

## Related incident

Arbitrum outage Dec 2023; audit findings (incidents.md §4, §5).

## Parameters

`sequencer.proxy` per L2 network.

## Known false positives and tuning

None.
