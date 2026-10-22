# Research notes (Phase 0)

Status: draft for review, 2026-10-22.

This document records what `oraclewatch` assumes about Chainlink Data Feeds, where each
assumption comes from, and how it was checked. Anything that could not be confirmed from
official documentation, verified contract source, or on-chain reads is marked
**unconfirmed** and the checks that depend on it are `experimental`.

`oraclewatch` is a consumer-side monitor. Chainlink operates its own monitoring of the
feeds; this tool exists because a protocol that consumes a feed carries the risk of the
assumptions its contracts make (freshness, bounds, sequencer status, deviation), and
should validate those assumptions independently and continuously. It is not affiliated
with Chainlink Labs.

## Contents

1. [Data feed model](#1-data-feed-model)
2. [L2 sequencer uptime feeds](#2-l2-sequencer-uptime-feeds)
3. [minAnswer / maxAnswer](#3-minanswer--maxanswer)
4. [On-chain verification](#4-on-chain-verification)
5. [Proposed initial feed list](#5-proposed-initial-feed-list)
6. [Reference price sources](#6-reference-price-sources)
7. [RPC endpoints and polling budget](#7-rpc-endpoints-and-polling-budget)
8. [Check catalog](#8-check-catalog)
9. [Open questions](#9-open-questions)
10. [Sources](#10-sources)

---

## 1. Data feed model

### 1.1 Interface

Consumers read a **proxy** through `AggregatorV3Interface` [S1]:

| Function | Returns | Notes |
|---|---|---|
| `decimals()` | `uint8` | Fixed-point precision of `answer`. |
| `description()` | `string` | Description of the aggregator the proxy points to, e.g. `"ETH / USD"`. |
| `version()` | `uint256` | Type of aggregator the proxy points to. All feeds verified here return `6`. |
| `latestRoundData()` | `(uint80 roundId, int256 answer, uint256 startedAt, uint256 updatedAt, uint80 answeredInRound)` | Main read. |
| `getRoundData(uint80)` | same tuple | Historical read by proxy round id. |

Return values of `latestRoundData()` per the API reference [S1] and the verified
aggregator source [S7, S8]:

| Field | Meaning in current (OCR2 / DualAggregator) aggregators |
|---|---|
| `roundId` | Proxy round id: `(phaseId << 64) \| aggregatorRoundId` [S2]. |
| `answer` | Median of the latest accepted report, fixed point with `decimals()`. |
| `startedAt` | `observationsTimestamp` of the report: when the observations were made off-chain [S8, NatSpec of `latestRoundData`]. |
| `updatedAt` | `block.timestamp` of the block that included the report on-chain. |
| `answeredInRound` | **Deprecated** [S1]. In OCR2/DualAggregator it is set equal to `roundId` [S8]; every read in §4 confirms `answeredInRound == roundId`. |

Consequence for checks: the classic audit recommendation `answeredInRound >= roundId`
is vacuous on current aggregators and is **not** implemented as a check. Freshness has
to be judged from `updatedAt`.

Deprecated functions that must not be used: `latestAnswer`, `getAnswer`,
`getTimestamp`, `latestRound`, `latestTimestamp` [S1].

### 1.2 Proxy, aggregator, phases

- The proxy forwards to the current aggregator; this is what allows upgrades without
  changing consumer contracts [S2]. `aggregator()` on the proxy returns the current
  aggregator address; `phaseId()` returns the current phase. Both were read on every
  verified feed (§4).
- Each aggregator upgrade increments the phase. `roundId` on the proxy keeps increasing
  across phases because the phase sits in the high bits [S2].
- Aggregator round ids are not guaranteed to be sequential across the proxy ("may not be
  monotonic", "not every ID is valid") [S2]. Within a phase, consecutive aggregator round
  ids were valid in every sample taken (40 rounds per feed, §4.3), but this is an
  observation, not a guarantee.
- `startedAt == 0` or `updatedAt == 0` means the round is incomplete and must not be
  used [S2].
- **Observed, not documented:** on OCR2 aggregators, `getRoundData` for a round id that
  does not exist (aggregator round 0 of phase 4 on Ethereum USDC/USD) returned
  `(answer=0, startedAt=0, updatedAt=0)` **without reverting**. The historical-data page
  says to stop when a call reverts [S2], which matches older aggregators. Any code that
  walks history must treat a zero `updatedAt` as "no such round".

### 1.3 Heartbeat and deviation threshold

A feed publishes a new round when the off-chain value deviates from the on-chain value
by more than the **deviation threshold**, or when the **heartbeat** elapses since the
last update, whichever comes first [S2, S3]. Both values are published per feed and per
network on the addresses page [S4]; the page renders them from the JSON data files
listed in §10 [S5].

These are off-chain operating parameters: they cannot be read from the contract.
`oraclewatch` takes them from configuration (copied from the data file, with source
and date), and §4.3 measures the real update cadence to calibrate tolerances.

### 1.4 SVR feeds (Smart Value Recapture)

Several networks list "SVR" variants of a feed next to the standard one [S6]:

- Same `AggregatorV3Interface`; only the address changes.
- The aggregator is a `DualAggregator` that receives the same report through two paths:
  public (standard proxy) and private (SVR auction). If the private path fails, the SVR
  path falls back to the standard price after a configurable delay (`cutoffTime`) [S6, S9].
- **`latestRoundData()` on a `DualAggregator` depends on `msg.sender`** (which proxy is
  calling) and on `block.timestamp` [S9, `_getLatestRound`]. The monitor must therefore
  always read through the proxy a consumer uses, never the aggregator directly, and at a
  pinned block.
- Variants: "Aave SVR" is dedicated to Aave; "Shared SVR" is the canonical shared
  variant and "the recommended feeds for new integrations" [S6]; "SVR-Backup (Legacy)"
  should not be used for new integrations.
- On Base the data file lists **no** plain (non-SVR) ETH/USD or BTC/USD entry; only SVR
  variants. The proposal uses the Shared SVR entries (`eth-usd-shared-svr-2`,
  `btc-usd-shared-svr-2`) and their `proxyAddress`. The data file also has a
  `secondaryProxyAddress` for these entries. **Unconfirmed:** which of the two proxies
  the docs consider the "SVR" address versus the "Standard" one; see §9.

### 1.5 Feed risk categories

The docs classify feeds by market pricing risk (low, medium, high, very high, new token,
custom, deprecating, unrated) and recommend freshness checks, fallback behaviour,
monitoring against independent references and circuit breakers for extreme events
[S3]. All feeds proposed here are in the **low** category. Feed category is kept in the
feed config so that future checks can apply stricter defaults to riskier feeds.

## 2. L2 sequencer uptime feeds

From the official page [S10]:

- `answer == 0`: sequencer **up**; `answer == 1`: sequencer **down**.
- `startedAt`: timestamp of the last status change.
- The example consumer waits a grace period after recovery: `GRACE_PERIOD_TIME = 3600`
  (1 hour). The page gives no other recommended value.
- On Arbitrum, `startedAt` is `0` before the uptime contract is initialised; on the other
  networks it is set at construction and is never 0.
- During downtime on OP-stack chains, no L2 transactions execute; messages queue on L1
  and are processed in order when the sequencer returns. On Arbitrum, messages wait in
  the delayed inbox and are processed before new transactions after recovery. Either
  way, the first blocks after recovery can carry stale prices and allow liquidations
  that users could not prevent, which is why the grace period exists.
- Chainlink "is no longer expanding L2 Sequencer Uptime Feeds to additional networks"
  [S10].

Verified on-chain (§4): Arbitrum `0xFdB631F5EE196F0ed6FAa767959853A9F217697D`
(`ArbitrumSequencerUptimeFeed 1.0.0`), Base `0xBCF85224fc0756B9Fa45aA7892530B47e10b6433`
and OP `0x371EAD81c9102C9BF4874A9075FFFf170F2Ee389` (`OptimismSequencerUptimeFeed 1.0.0`),
all `decimals() = 0`, `description() = "L2 Sequencer Uptime Status Feed"`, `answer = 0`.
The addresses on the L2 page and in the network data files are identical.

Observed: on Base and OP, `updatedAt` is refreshed while `startedAt` stays at the last
status change (e.g. OP `startedAt` 2026-07-07T18:21:35Z, `updatedAt`
2026-10-21T19:01:43Z). The sequencer feed has no published heartbeat, so **the staleness
check does not apply to it**; only `answer` and `startedAt` matter.

## 3. minAnswer / maxAnswer

### 3.1 What the docs say

The API reference marks `minAnswer`/`maxAnswer` as "no longer used on most Data Feeds"
[S1]. That wording is about whether the bounds are set to meaningful values, not about
whether the contract enforces them.

### 3.2 What the deployed code does (confirmed)

Verified source of the deployed aggregators (Sourcify, exact/full match):

- `AccessControlledOCR2Aggregator` (e.g. Ethereum ETH/USD aggregator
  `0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5`, Ethereum USDC/USD aggregator
  `0x54bCC589d9743E521c64233706FC8cB36D275b07`) [S7]:
  `require(minAnswer <= median && median <= maxAnswer, "median is out of min-max range");`
  inside `transmit`.
- `DualAggregator` (Base ETH/USD aggregator `0x05c84a58FE042275b37db038bAAcD15F410c7bB0`)
  [S9]: `if (i_minAnswer > median || median > i_maxAnswer) revert MedianIsOutOfMinMaxRange();`
- The same check exists in `libocr` `contract2/OCR2Aggregator.sol` at commit
  `618b5bf7f342075a81ca1273a04abce15529a101` [S8].

Consequence, and the precise mental model the checks use: **the aggregator does not
clamp the answer to the bound. A report whose median is out of range reverts, so the
feed stops updating and keeps returning the last in-range answer.** Out of range shows up
as a price frozen near a bound that keeps getting older, while the market moves away
from it. This matches the LUNA/USD incident, where the feed showed about $0.107 while
the reported `minAnswer` was $0.10 (see `docs/incidents.md`). That 2022 feed ran an
older OCR aggregator whose source was not verified for this document.

### 3.3 Observed bounds

| Feeds | `minAnswer` | `maxAnswer` | Effective? |
|---|---|---|---|
| ETH/USD, BTC/USD (all 4 networks) | `1` (1e-8 USD) | `95780971304118053647396689196894323976171195136475135` | No: nominal values. |
| USDC/USD (all 4 networks) | `1` | `105000000` (= **1.05 USD**) | **Yes, upper bound.** |

The USDC/USD feeds have a real upper bound at $1.05. If USDC traded above that, these
feeds would stop updating at the last answer ≤ $1.05. The lower bound is nominal.

### 3.4 Decision

- The circuit-breaker proximity check is `stable` **only** for aggregator types whose
  verified source was read: `AccessControlledOCR2Aggregator 1.0.0` and
  `DualAggregator 1.0.0`. For any other `typeAndVersion()` it reports `unknown` and the
  check is `experimental`.
- A bound is treated as nominal (check reports "not applicable") when
  `minAnswer <= 1` for the lower side, or `maxAnswer` is at least 1e30 times the current
  answer for the upper side. This rule is a heuristic and is documented as such.
- Bounds are read on-chain from the current aggregator (via `aggregator()` on the
  proxy), cached, and re-read when the aggregator changes. They are never configured
  by hand.

## 4. On-chain verification

### 4.1 Method

1. Addresses were taken only from the official data files that the addresses page loads
   [S5] and, for sequencer feeds, also from the L2 page [S10]. No address was typed
   by hand; `configs/feeds/*.yaml` is generated from those files plus the
   on-chain results, and the generator asserts that they agree.
2. For each network, two independent RPC endpoints were queried. Both had to report the
   expected `eth_chainId`. A block was pinned at `min(head_A, head_B) - 5` and **every**
   call was made at that block number on both endpoints.
3. On each proxy: `description()`, `decimals()`, `version()`, `aggregator()`,
   `phaseId()`, `latestRoundData()`. On the returned aggregator: `typeAndVersion()`,
   `minAnswer()`, `maxAnswer()`.
4. Pass criteria: `description` and `decimals` equal to the data file; `aggregator()`
   equal to the data file's `contractAddress`; EIP-55 checksum of the address
   unchanged; both RPCs returned byte-identical results.

Function selectors were computed by go-ethereum from the ABI (not typed by hand), e.g.
`latestRoundData()` = `0xfeaf968c`, `aggregator()` = `0x245a7bfc`,
`typeAndVersion()` = `0x181f5a77`.

### 4.2 Result

All 12 price feeds and 3 sequencer uptime feeds passed every criterion on
2026-10-22 (UTC). Pinned blocks: Ethereum 26245361, Arbitrum 517104692,
Base 52924574, OP 158519860. The raw output is in
[`docs/evidence/onchain-verification-2026-10-22.txt`](evidence/onchain-verification-2026-10-22.txt).
In the sequencer section of that file, `aggregatorMatchesDocs: false` only means that
the data file does not publish an aggregator address for sequencer feeds, so there was
nothing to compare against.

### 4.3 Observed update cadence

`getRoundData` over the 40 rounds before the latest one, largest gap between consecutive
`updatedAt` values divided by the published heartbeat:

| Network | Feed | Heartbeat | Max observed gap | Ratio |
|---|---|---|---|---|
| Ethereum | ETH/USD | 3600 s | 3636 s | 1.01 |
| Ethereum | BTC/USD | 3600 s | 3636 s | 1.01 |
| Ethereum | USDC/USD | 82800 s | 82836 s | 1.00 |
| Arbitrum | ETH/USD | 1755 s | 331 s | 0.19 (deviation-driven) |
| Arbitrum | BTC/USD | 1755 s | 330 s | 0.19 (deviation-driven) |
| Arbitrum | USDC/USD | 255 s | 272 s | **1.07** |
| OP | ETH/USD | 1200 s | 1212 s | 1.01 |
| OP | BTC/USD | 1200 s | 1212 s | 1.01 |
| OP | USDC/USD | 86400 s | 86426 s | 1.00 |
| Base | ETH/USD (Shared SVR) | 1200 s | 1232 s | 1.03 |
| Base | BTC/USD (Shared SVR) | 1200 s | 1230 s | 1.02 |
| Base | USDC/USD | 86400 s | 86430 s | 1.00 |

Takeaways:

- A heartbeat is a target, not a hard upper bound: updates land a few seconds to ~30 s
  after it (round trigger, transmission, block inclusion).
- A purely relative tolerance is wrong for short heartbeats: 10% of 255 s is 25.5 s,
  and the Arbitrum USDC/USD feed already reached +17 s. The staleness check therefore
  uses `max_age = heartbeat + max(heartbeat × tolerance, min_slack)` with defaults
  `tolerance = 10%`, `min_slack = 60s`.

## 5. Proposed initial feed list

All values from the official data files [S5], verified as in §4. The generated
files are `configs/feeds/{ethereum,arbitrum,base,optimism}.yaml`.

| Network | Pair | Proxy | Dec. | Heartbeat | Deviation | Aggregator type | Notes |
|---|---|---|---|---|---|---|---|
| Ethereum (1) | ETH/USD | `0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419` | 8 | 3600 s | 0.5% | OCR2 | |
| Ethereum (1) | BTC/USD | `0xF4030086522a5bEEa4988F8cA5B36dbC97BeE88c` | 8 | 3600 s | 0.5% | OCR2 | |
| Ethereum (1) | USDC/USD | `0x8fFfFfd4AfB6115b954Bd326cbe7B4BA576818f6` | 8 | 82800 s | 0.25% | OCR2 | maxAnswer $1.05 |
| Arbitrum (42161) | ETH/USD | `0x639Fe6ab55C921f74e7fac1ee960C0B6293ba612` | 8 | 1755 s | 0.05% | OCR2 | |
| Arbitrum (42161) | BTC/USD | `0x6ce185860a4963106506C203335A2910413708e9` | 8 | 1755 s | 0.05% | OCR2 | |
| Arbitrum (42161) | USDC/USD | `0x50834F3163758fcC1Df9973b6e91f0F0F0434aD3` | 8 | 255 s | 0.1% | OCR2 | maxAnswer $1.05 |
| Base (8453) | ETH/USD | `0x50015f8b17fb2C290Dde41fDc246ed0dcEE93a8b` | 8 | 1200 s | 0.15% | DualAggregator | Shared SVR |
| Base (8453) | BTC/USD | `0xC01502b887D09B0f329981764268fC32a94cB453` | 8 | 1200 s | 0.1% | DualAggregator | Shared SVR |
| Base (8453) | USDC/USD | `0x7e860098F58bBFC8648a4311b374B1D669a2bc6B` | 8 | 86400 s | 0.3% | OCR2 | maxAnswer $1.05 |
| OP (10) | ETH/USD | `0x13e3Ee699D1909E989722E753853AE30b17e08c5` | 8 | 1200 s | 0.15% | OCR2 | |
| OP (10) | BTC/USD | `0xD702DD976Fb76Fffc2D3963D037dfDae5b04E593` | 8 | 1200 s | 0.1% | OCR2 | |
| OP (10) | USDC/USD | `0x16a9FA2FDa030272Ce99B29CF780dFA30361E0f3` | 8 | 86400 s | 0.1% | OCR2 | maxAnswer $1.05 |

Sequencer uptime feeds: Arbitrum, Base and OP (addresses in §2).

Excluded for v1: Polygon (no sequencer feed, which removes the L2-specific checks; it can
be added later by generating one more file). Data for Polygon was retrieved and is
easy to add.

Why these feeds: ETH/USD and BTC/USD are the most widely consumed collateral feeds on
every network; USDC/USD has a long heartbeat on most networks (up to 24 h), which makes
it a good test for staleness logic, and it is the only feed set here with an effective
`maxAnswer`, which exercises the circuit-breaker check. Running the same pairs on four
networks enables the cross-chain divergence check.

## 6. Reference price sources

Requirements: public, unauthenticated, real **USD** quote (not USDT), reachable from
Brazil and from an OCI region, price returned as a string.

Probed on 2026-10-22 from Brazil (Cloudflare edge GRU), all HTTP 200:

| Source | Endpoint | Pairs available | Timestamp in response | Public rate limit |
|---|---|---|---|---|
| Coinbase Exchange | `GET https://api.exchange.coinbase.com/products/{ETH-USD}/ticker` | ETH-USD, BTC-USD. **No USDC-USD** (404): Coinbase treats USDC as 1:1 with USD, so it cannot be a USDC reference. | `time` (last trade, RFC 3339) | 10 req/s per IP, bursts of 15 [S11] |
| Kraken | `GET https://api.kraken.com/0/public/Ticker?pair={XETHZUSD}` | ETH, BTC, USDC vs USD | none | Counter, max 15, decays 0.33/s, +1 per call [S12] |
| Bitstamp | `GET https://www.bitstamp.net/api/v2/ticker/{ethusd}/` | ETH, BTC, USDC vs USD | `timestamp` (Unix seconds; meaning not documented) | 400 req/s, 10,000 req per 10 min [S13] |
| Gemini | `GET https://api.gemini.com/v1/pubticker/{ethusd}` | ETH, BTC, USDC vs USD | `volume.timestamp` (ms, minute-bucketed in observation; meaning not documented) | **unconfirmed** (docs page did not list it) |

Decisions:

- Default sources: **Coinbase, Kraken, Bitstamp** for ETH/BTC; **Kraken, Bitstamp,
  Gemini** for USDC. Gemini is optional for ETH/BTC.
- Price used: **mid = (bid + ask) / 2**, parsed from strings into `big.Rat`. Mid is more
  robust than `last` on thin books and is available from all four (Kraken `a[0]`/`b[0]`).
- Freshness: only Coinbase (`time`) and Bitstamp (`timestamp`) expose a usable time; for
  the others the fetch time is used. Documented as a limitation.
- Polling once per cycle (default 60 s) per pair is far below every documented limit.
- **USD vs USDT:** many exchanges quote against USDT. USDT is not USD; at probe time
  Coinbase USDT-USD was 0.99906, about 0.1% off. During a stablecoin depeg the gap can
  be several percent (USDC fell to about $0.88 in March 2023 [S14]). Mixing USDT quotes
  into a USD reference would create false deviation alerts, or hide real ones, exactly
  when they matter. v1 accepts only USD-quoted markets; config validation rejects
  USDT/USDC quote currencies for the reference.
- Spread between sources > `max_source_spread` (default 1%), or fewer than
  `min_sources` (default 2) answering: the reference is marked **untrusted**, the
  deviation check returns `unknown` (no alert), and a separate `FW-REF-001` warning is
  raised.

## 7. RPC endpoints and polling budget

Probed on 2026-10-22, all returned the correct `eth_chainId` and a current head:

| Network | Public endpoints that worked |
|---|---|
| Ethereum | `ethereum-rpc.publicnode.com`, `eth.drpc.org` (`eth.llamarpc.com` returned 525; `cloudflare-eth.com` refused `eth_blockNumber`) |
| Arbitrum | `arbitrum-one-rpc.publicnode.com`, `arb1.arbitrum.io/rpc`, `arbitrum.drpc.org` |
| Base | `base-rpc.publicnode.com`, `mainnet.base.org`, `base.drpc.org` |
| OP | `optimism-rpc.publicnode.com`, `mainnet.optimism.io`, `optimism.drpc.org` |

The official Arbitrum public RPC states "No uptime, latency, or rate-limit guarantees"
and is meant for development and low-volume reads [S15]. Published limits for the
other public endpoints were not found, so the design assumes **no guarantees** for any
free endpoint and requires at least two RPCs per network (ideally from different
providers, one of them a keyed free tier such as Alchemy, Infura, QuickNode or dRPC).

Per network and cycle, with 2 RPCs and 3 feeds:

- per RPC: `eth_blockNumber` (1) + header of pinned block (1) + `latestRoundData` per
  feed (3) + sequencer `latestRoundData` (1) = 6 calls;
- metadata (`decimals`, `description`, `version`, `aggregator`, `phaseId`) every
  10th cycle, and bounds only when the aggregator changes.

That is about 12 calls per minute per network at `poll_interval: 60s`, or 0.2 req/s,
comfortably inside every free tier. The `eth_call`s can be sent in one JSON-RPC batch
per endpoint (go-ethereum `BatchCallContext`), which keeps HTTP round trips low.
**Multicall3 is not needed for v1** (it would also hide which RPC returned which value
if mixed with the disagreement check); this will be recorded in an ADR.

Minimum sensible `poll_interval` for the shortest heartbeat in the list (255 s): 60 s
gives at most one minute of detection latency. The default is 60 s, with 15 s as the
validated minimum.

## 8. Check catalog

Statuses: `stable` checks are on by default; `experimental` checks are off by default.
Severity is the highest the check can emit; most checks emit `warning` before
`critical`. All comparisons use `big.Int`/`big.Rat`; `float64` is only used to export
metrics.

Common input of every cycle: the network's pinned block `B` (number, hash, timestamp
`T_B`), the wall clock `now` (injected), and for each feed the tuple read at `B`.

| ID | Title | Max sev. | Exact logic | Parameters (default) | Rationale | Incident | Status |
|---|---|---|---|---|---|---|---|
| `FW-STALE-001` | Feed stale | critical | `age = T_B - updatedAt`. `max_age = heartbeat + max(heartbeat × tolerance, min_slack)`. Warning if `age > max_age`; critical if `age > critical_factor × max_age`. Uses block time, not wall time, so that a halted chain is reported by `FW-CHAIN-001` instead of as N stale feeds. Not applied to sequencer feeds. | `tolerance` 10%, `min_slack` 60s, `critical_factor` 2 | Consumers that skip freshness checks act on old prices; a frozen feed is also the visible symptom of an out-of-range report (§3.2). | LUNA/USD 2022; sequencer outages | stable |
| `FW-CHAIN-001` | Chain head stalled / RPC lagging | critical | `lag = now - T_head` for each RPC's head. Warning if `lag > max_head_lag`; if all RPCs of a network lag, the chain is considered halted (critical) and feed checks of that network report `unknown`. | `max_head_lag` 5m (Ethereum), 2m (L2s) | Separates "the feed is old" from "my view of the chain is old"; protects against an RPC stuck 1,000 blocks behind. | Arbitrum outage 2023-12-15 | stable |
| `FW-INVALID-001` | Invalid round data | critical | Critical if any: `answer <= 0`; `updatedAt == 0`; `startedAt == 0`; `updatedAt > T_B` (impossible: `updatedAt` is the block time of an earlier block, so this means a lying or broken RPC); `roundId` lower than the last `roundId` seen for the same proxy (state went backwards). Warning if `startedAt > updatedAt + skew`. `answeredInRound` is intentionally not checked (§1.1). | `skew` 60s | Audits repeatedly flag missing `answer > 0` and incomplete-round checks. | Audit findings (incidents.md §4) | stable |
| `FW-DEV-001` | Deviation from market reference | critical | Only if the reference is trusted. `dev = |oracle - ref| / ref` (`big.Rat`). Warning if `dev > warn_above`, critical if `dev > critical_above`. Validation enforces `warn_above >= 2 × deviation_threshold`, because the oracle may legitimately lag by up to its threshold until the next round, and SVR feeds may add a configurable delay. Alerts need persistence (`for:` in the alert manager). | `warn_above` 2%, `critical_above` 5% | Detects a frozen or wrong feed while the market moves (LUNA), and wrong-answer events. | LUNA/USD 2022 | stable |
| `FW-REF-001` | Reference degraded | warning | Fewer than `min_sources` answered, or `(max - min) / median > max_source_spread`. Suppresses `FW-DEV-001` for that pair. | `min_sources` 2, `max_source_spread` 1% | One bad exchange must not trigger, or mask, a deviation alert. | — | stable |
| `FW-SEQ-001` | L2 sequencer down | critical | Sequencer feed `answer == 1`. `answer` not in {0,1}, or `startedAt == 0` on Arbitrum, is `FW-INVALID-001` for the sequencer feed. | — | Prices cannot update while the sequencer is down; liquidations on recovery use stale prices. | Arbitrum outage 2023-12-15; audits | stable |
| `FW-SEQ-002` | L2 sequencer grace period | warning | `answer == 0` and `T_B - startedAt <= grace_period` (same comparison as the official example). | `grace_period` 1h | Consumers following the docs reject prices during this window; the operator should know. | Same as above | stable |
| `FW-CB-001` | Near circuit-breaker bound | critical | Only for verified aggregator types (§3.4) and non-nominal bounds. `dist_min = (answer - minAnswer) / answer`, `dist_max = (maxAnswer - answer) / answer`. Warning if `dist < warn_within`; critical if `dist < critical_within`, or if `answer` is within `critical_within` of a bound **and** `FW-STALE-001` or `FW-DEV-001` is firing (frozen at the bound). | `warn_within` 2%, `critical_within` 0.5% | The aggregator rejects out-of-range reports, so the feed freezes at the last in-range answer. | LUNA/USD 2022 | stable for OCR2/DualAggregator 1.0.0; experimental otherwise |
| `FW-AGG-001` | Aggregator changed | info | `aggregator()` or `phaseId()` differs from the previous read for the proxy. Triggers a metadata and bounds refresh. | — | Usually a legitimate upgrade, but the consumer should know: bounds, decimals and behaviour can change with it. | — | stable |
| `FW-META-001` | Metadata drift | critical | `decimals()` differs from `expected_decimals` → critical (consumers that hard-code decimals misprice by powers of ten). `description()` differs from the configured one → warning. `version()` changed → info. | — | Integration assumptions break silently. | Moonwell cbETH 2026 (composition error, related class) | stable |
| `FW-XCHAIN-001` | Cross-chain divergence | warning | For the same pair on networks A and B, each read at its own pinned block: `dev = |a - b| / min(a, b)`. Warning if `dev > threshold_A + threshold_B + margin`. Skipped if either side is stale or its chain is stalled. | `margin` 1% | Divergence beyond what both deviation thresholds allow points at a stuck or misbehaving feed on one network. | — | stable |
| `FW-RPC-001` | RPC disagreement | critical | Pin `B = min(head_i) - confirmations`. Read every feed at `B` on every healthy RPC. If block **hashes** at `B` differ → warning `FW-RPC-002` (fork/reorg view), comparison skipped. If hashes match but any returned tuple differs byte for byte → critical. | `confirmations` 2 (Ethereum), 10 (L2s, pending ADR) | Never trust a single RPC: a compromised or misconfigured RPC can feed the monitor false data. Same block plus same hash means honest nodes must agree exactly. | — | stable |
| `FW-RPC-002` | RPC fork view | warning | See above. | — | — | — | stable |
| `FW-RPC-003` | Wrong chain | critical | `eth_chainId` differs from the configured `chain_id`. The endpoint is excluded until restart. Checked at start-up and every `chainid_recheck` cycles. | `chainid_recheck` 60 | Protects against misconfiguration and endpoints that change what they serve. | — | stable |
| `FW-SELF-001` | Monitor blind | critical | No successful read for a network for `blind_after`; or all reference sources failing for `blind_after`. Optional external dead man's switch: an HTTP ping after each successful cycle (e.g. healthchecks.io), so a dead process is detected from outside. | `blind_after` 3 × `poll_interval` | The worst failure of a monitor is a silent one (false negative). | — | stable |
| `FW-JUMP-001` | Abrupt round-to-round change | warning | `|answer_n - answer_{n-1}| / answer_{n-1} > max_jump` between consecutive rounds of the same phase. | `max_jump` 20% | Catches implausible values for which no market reference exists (exchange-rate feeds). | Moonwell wrsETH 2025 | experimental |

Explicitly **not** checks, with reasons:

- `answeredInRound >= roundId`: deprecated field, always equal on current aggregators.
- "Answer equals minAnswer": the aggregator never stores a value outside the range, and
  out-of-range reports revert instead of clamping, so equality is not the signal; a
  frozen price near a bound is (`FW-CB-001` + `FW-STALE-001`).

## 9. Open questions

1. **SVR proxies on Base.** The data file lists `proxyAddress` and
   `secondaryProxyAddress` for SVR entries. `DualAggregator` treats the secondary proxy
   specially (it may return the private, auction-path round). The Moonwell post-mortem
   lists `0x71041dddad3595F9CEd3DcCFBe3D1F4b0a16Bb70` (the `secondaryProxyAddress` of
   `eth-usd-shared-svr-2`) as the ETH/USD feed it reads. Proposal: monitor
   `proxyAddress` by default, allow the operator to choose the proxy their protocol
   actually reads, and document the difference. Needs your decision.
2. **Gemini rate limit** could not be confirmed from its docs; it stays optional.
3. **Confirmations on L2s.** Reorgs on OP-stack and Arbitrum are rare at the unsafe head,
   but public RPCs may disagree for a few blocks. 10 blocks is a conservative default
   (about 2–20 s); to be confirmed against the chaos tests and recorded in an ADR.

## 10. Sources

Official documentation and data (retrieved 2026-10-22):

- [S1] API reference: https://docs.chain.link/data-feeds/api-reference
- [S2] Historical data / round ids: https://docs.chain.link/data-feeds/historical-data
- [S3] Selecting quality data feeds: https://docs.chain.link/data-feeds/selecting-data-feeds
- [S4] Price feed addresses: https://docs.chain.link/data-feeds/price-feeds/addresses
- [S5] Data files loaded by [S4]:
  https://reference-data-directory.vercel.app/feeds-mainnet.json,
  https://reference-data-directory.vercel.app/feeds-ethereum-mainnet-arbitrum-1.json,
  https://reference-data-directory.vercel.app/feeds-ethereum-mainnet-base-1.json,
  https://reference-data-directory.vercel.app/feeds-ethereum-mainnet-optimism-1.json,
  https://reference-data-directory.vercel.app/feeds-matic-mainnet.json
- [S6] SVR feeds: https://docs.chain.link/data-feeds/svr-feeds
- [S10] L2 sequencer uptime feeds: https://docs.chain.link/data-feeds/l2-sequencer-feeds

Verified contract source:

- [S7] `AccessControlledOCR2Aggregator`, Sourcify exact match:
  https://sourcify.dev/server/v2/contract/1/0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5
  and https://sourcify.dev/server/v2/contract/1/0x54bCC589d9743E521c64233706FC8cB36D275b07
- [S8] libocr `contract2/OCR2Aggregator.sol` @ `618b5bf7f342075a81ca1273a04abce15529a101`:
  https://github.com/smartcontractkit/libocr/blob/618b5bf7f342075a81ca1273a04abce15529a101/contract2/OCR2Aggregator.sol
- [S9] `DualAggregator`, Sourcify match (solc 0.8.24):
  https://sourcify.dev/server/v2/contract/8453/0x05c84a58FE042275b37db038bAAcD15F410c7bB0

Exchanges and infrastructure:

- [S11] Coinbase Exchange rate limits: https://docs.cdp.coinbase.com/exchange/rest-api/rate-limits
- [S12] Kraken REST rate limits: https://docs.kraken.com/api/docs/guides/spot-rest-ratelimits
- [S13] Bitstamp API: https://www.bitstamp.net/api/
- [S14] The Block, USDC at about $0.90 after the SVB disclosure (March 2023):
  https://www.theblock.co/post/218993/usdc-and-dai-remain-at-about-0-90-following-circles-disclosure-of-funds-at-svb
- [S15] Arbitrum node providers / public RPC: https://docs.arbitrum.io/build-decentralized-apps/reference/node-providers
