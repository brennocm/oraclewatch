# FW-AGG-001: Aggregator changed

- **Maximum severity:** info
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

Info when `aggregator()` or `phaseId()` differs from the previous metadata read. Metadata and bounds are re-read after a change.

## Why it matters

Usually a legitimate upgrade, but bounds, decimals and behaviour may change with the aggregator; a consumer should know.

## Related incident

—

## Parameters

None. Metadata is read every `rpc.metadata_refresh` cycles (10).

## Known false positives and tuning

Restarting oraclewatch resets the baseline (state is in memory), so a change during downtime is not reported.
