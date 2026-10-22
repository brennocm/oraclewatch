# 2. RPC trust: pinned block, multiple endpoints, failover

Date: 2026-10-22
Status: accepted

## Context

Everything oraclewatch knows about a feed comes from JSON-RPC endpoints, usually run by
third parties. An endpoint can be down, rate limited, behind the chain, serving another
chain, on a short-lived fork, or lying. A monitor that trusts one endpoint inherits all of
these failure modes, and the worst of them (a lying or stale endpoint) produce false
negatives without any visible error.

Values read at different blocks can legitimately differ. Values read at the same block
with the same block hash cannot: the EVM is deterministic.

## Decision

1. **Several endpoints per network**, configured by name. Names are used in logs and
   metrics; URLs never leave the process (ADR on redaction, `internal/redact`).
2. **Chain ID check** of every endpoint at start-up and every `rpc.chain_id_recheck`
   cycles. An endpoint that serves another chain is disabled until restart and raises
   `FW-RPC-003`.
3. **One pinned block per network and cycle.** Each available endpoint reports its head;
   endpoints whose head is older than `max_head_lag` are excluded (`FW-CHAIN-001`), and
   the block is `min(remaining heads) - confirmations`. Every read of the cycle, on
   every endpoint, uses that block number (`eth_call` with an explicit block).
4. **Hash agreement.** Each endpoint returns the pinned header; endpoints whose hash
   differs from the majority (ties: configuration order) are excluded from the cycle and
   raise `FW-RPC-002`. Only endpoints that agree on the hash are compared for
   `FW-RPC-001`, where any byte difference in a feed read is critical.
5. **Per-call timeout** from `call_timeout`. **Retries** only for transient errors
   (timeouts, connection failures, HTTP 429/5xx, JSON-RPC `-32005`): at most 2 retries
   with exponential backoff and full jitter (200 ms, 400 ms, capped at 2 s). A JSON-RPC
   error such as `execution reverted` is an answer, not a failure, and is neither
   retried nor counted against the endpoint.
6. **Circuit breaker per endpoint.** Three consecutive failed calls open it for 30 s,
   doubling up to 5 min while probes keep failing. A success closes it. Cancellation by
   the caller (shutdown, cycle deadline) is never counted.
7. **Failover is implicit**: the cycle uses every endpoint that is available and agrees,
   and the first in configuration order is preferred for single reads. There is no
   "active/standby" state to get stuck in.

## Alternatives

- **Single endpoint with fallback on error.** Simple, but a lying or lagging endpoint
  never errors, so it is never replaced.
- **Read `latest` on each endpoint.** Results are not comparable, so disagreement
  detection is impossible, and a feed can look stale on one endpoint and fresh on
  another in the same cycle.
- **Quorum of N endpoints for every value.** Stronger, but free tiers make three or more
  independent endpoints per network unrealistic for most operators; the pinned block plus
  hash agreement plus exact comparison gets most of the benefit with two.
- **Multicall3 to batch reads.** See ADR 0005.

## Consequences

- Detection latency gains `confirmations` blocks (2 on Ethereum, about 24 s; 10 on the
  L2s, a few seconds).
- A lagging endpoint drags nothing back; it is excluded and reported.
- With two endpoints that disagree, oraclewatch cannot tell which one is right. It reports
  the disagreement as critical and keeps reading from the configuration-order winner;
  adding a third endpoint resolves ties by majority.
- Half-open breakers may let a few concurrent calls through at once; acceptable at this
  request rate.
