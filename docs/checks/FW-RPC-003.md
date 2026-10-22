# FW-RPC-003: Wrong chain

- **Maximum severity:** critical
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

`eth_chainId` differs from `chain_id` at start-up or at a periodic re-check; the endpoint is disabled until restart.

## Why it matters

Protects against configuration mistakes (an Ethereum URL under Arbitrum) and endpoints that change what they serve.

## Related incident

—

## Parameters

`rpc.chain_id_recheck` (60 cycles).

## Known false positives and tuning

None.
