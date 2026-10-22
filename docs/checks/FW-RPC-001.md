# FW-RPC-001: RPC disagreement

- **Maximum severity:** critical
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

Every reader endpoint (same pinned block number and same block hash) reads `latestRoundData`; any byte difference in any field is critical. Endpoints that errored are ignored. Fewer than two answers is Unknown.

## Why it matters

The EVM is deterministic: with the same block hash, honest nodes must agree exactly. A difference means a broken, misconfigured or lying RPC, which would otherwise produce false negatives silently.

## Related incident

—

## Parameters

`rpc.compare_endpoints` (true).

## Known false positives and tuning

None expected with the hash check in place; if it fires, distrust the cycle and inspect each endpoint's values in the alert details.
