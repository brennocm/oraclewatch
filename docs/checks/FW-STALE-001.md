# FW-STALE-001: Feed stale

- **Maximum severity:** critical
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

`age = blockTime(pinned block) - updatedAt`, `max_age = heartbeat + max(heartbeat × tolerance, min_slack)`.
Warning if `age > max_age`; critical if `age > critical_factor × max_age`. Equality does not fire.
Unknown when the chain is stalled (reported once by FW-CHAIN-001), when the read failed, or when `updatedAt` is zero (FW-INVALID-001).

## Why it matters

A consumer that does not check freshness acts on an old price. A frozen feed is also the visible symptom of reports being rejected by the aggregator's min/max bounds (`docs/research.md` §3.2).

## Related incident

LUNA/USD, May 2022 (incidents.md §1); sequencer outages (§4).

## Parameters

`staleness.tolerance` (10%), `staleness.min_slack` (60s), `staleness.critical_factor` (2), plus the feed's `heartbeat`.

## Known false positives and tuning

Updates land a few seconds after the heartbeat (up to +7% observed on a 255 s heartbeat), which `min_slack` covers. A wrong `heartbeat` in the config is the most common cause: take it from `configs/feeds/*.yaml`. Feeds with 24 h heartbeats legitimately go a day without updates; the check is relative to the heartbeat, so this is not a false positive.
