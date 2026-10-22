# FW-CB-001: Near circuit-breaker bound

- **Maximum severity:** critical
- **Status:** stable (for AccessControlledOCR2Aggregator 1.0.0 and DualAggregator 1.0.0)
- **Implementation:** `internal/checks/`

## What it checks

For verified aggregator types and non-nominal bounds: `dist = (answer - minAnswer) / answer` or `(maxAnswer - answer) / answer`, the smaller of the applicable ones. Warning if `dist < warn_within`; critical if `dist < critical_within`, or `dist < warn_within` while FW-STALE-001 or FW-DEV-001 fire. A lower bound `<= 1` or an upper bound `>= 1e30 × answer` is nominal. Other aggregator types report Unknown.

## Why it matters

`transmit()` reverts on a median outside the bounds, so the feed freezes at the last in-range answer instead of following the market. Verified in deployed source (Sourcify) for both aggregator types above.

## Related incident

LUNA/USD, May 2022.

## Parameters

`circuit_breaker.warn_within` (2%), `circuit_breaker.critical_within` (0.5%).

## Known false positives and tuning

USDC/USD has a real `maxAnswer` of $1.05: at peg the distance is about 4.8%, so the default 2% only warns above about $1.029. Do not raise `warn_within` above 5% for those feeds or the check fires permanently.
