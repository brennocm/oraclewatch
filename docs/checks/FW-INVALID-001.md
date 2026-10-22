# FW-INVALID-001: Invalid round data

- **Maximum severity:** critical
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

Critical if `answer <= 0`, `updatedAt == 0`, `startedAt == 0`, `updatedAt` later than the pinned block's timestamp, or `roundId` lower than the last one seen for the proxy while reading at the same or a later block. Warning if `startedAt > updatedAt + skew`.
`answeredInRound` is not checked: it is deprecated and equals `roundId` on OCR2 and DualAggregator.

## Why it matters

These are the checks auditors most often find missing in consumer contracts. `updatedAt` after the block it was read at, or a round going backwards, cannot happen with an honest RPC.

## Related incident

Recurring audit findings (incidents.md §5).

## Parameters

`invalid.skew` (60s).

## Known false positives and tuning

If the pinned block moves backwards (for example a new, slightly behind endpoint joins), the round check is skipped rather than raising a false alarm. A legitimately zero-or-negative feed (some non-price feeds) should disable the check for that feed.
