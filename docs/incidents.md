# Oracle-dependency incidents

Real cases where depending on an oracle caused, or nearly caused, losses, and which
`oraclewatch` check would have flagged the condition. The rule for inclusion is a
primary source (post-mortem, protocol statement, official docs) or at least two
independent, reputable write-ups. Each entry also says what the tool would **not** have
prevented.

A monitor does not stop an exploit. It shortens the time between the condition
appearing and a human or an automated guardian acting (pausing a market, lowering caps).
In several cases below that time was the deciding factor.

---

## 1. LUNA/USD at the minimum bound: Venus (BNB Chain) and Blizz Finance (Avalanche), May 2022

**What happened.** During the Terra collapse, LUNA's market price fell below the
`minAnswer` of the LUNA/USD aggregator, reported as $0.10 [1]. Because out-of-range
reports are rejected, the feed stopped at the last accepted answer. Venus stated that
"the price on Venus was last listed at about $0.107 while the market price was $0.01",
and paused the protocol through its PauseGuardian [2][3]. Attackers bought LUNA at market
price, deposited it at the oracle price and borrowed against it. Venus reported about
$13.5M borrowed and an estimated loss of about $11.2M [2][3]; Blizz Finance lost about
$8.3M and could not react in time because of its timelock [1]. Chainlink's statement,
as summarised in [1] and [4], attributed it to "the unprecedented volatility across the
cryptocurrency markets" triggering the feed's automatic minimum-value circuit breaker.

The $0.107 shown versus the $0.10 bound is consistent with the deployed aggregator
behaviour documented in `research.md` §3.2: no clamping, the feed freezes at the last
in-range answer. (The 2022 aggregator was an older OCR version whose source was not
re-verified for this document.)

**Checks that would have fired.**

| Check | Why |
|---|---|
| `FW-CB-001` | The last accepted answer ($0.107) was within 7% of `minAnswer` ($0.10). With the defaults (warning within 2%), it would warn once an answer below about $0.102 was accepted, and go critical when the answer is near the bound while `FW-STALE-001` or `FW-DEV-001` fires. In this case the freeze happened at 7% from the bound, so the critical path is the combination with the next two checks, which is why `FW-CB-001` alone is not enough. |
| `FW-DEV-001` | Oracle $0.107 versus market about $0.01: deviation of about 970%. |
| `FW-STALE-001` | No new rounds once every report was out of range. |

**What it would not have done.** It cannot pause a market; it would have paged the
operator who holds the pause key. Blizz's timelock shows why the alert alone was not
enough there.

**Sources.**
1. rekt.news, "Blizz Finance, Venus Protocol – REKT": https://rekt.news/venus-blizz-rekt
2. The Record, "Collapse of Luna cryptocurrency leads to $11 million exploit on Venus Protocol": https://therecord.media/collapse-of-luna-cryptocurrency-leads-to-11-million-exploit-on-venus-protocol/
3. Halborn, "Explained: The LUNA Price Dip Exploits (May 2022)": https://halborn.com/explained-the-luna-price-dip-exploits-may-2022/
4. AMBCrypto, "Chainlink: How a price discrepancy resulted in millions lost from DeFi protocols" (summarises and links the official statement): https://ambcrypto.com/chainlink-how-a-price-discrepancy-resulted-in-millions-lost-from-defi-protocols/

---

## 2. wrsETH/ETH exchange-rate feed reports 1.65M ETH: Moonwell (Base), 2025-11-04

**What happened.** Moonwell priced wrsETH by multiplying a wrsETH/ETH feed
(`0xd7221b10FBBC1e1ba95Fd0B4D031C15f7F365296`) by ETH/USD. According to Moonwell's
post-mortem, the wrsETH/ETH oracle "erroneously reported that 1 wrsETH equals
1,649,934.60732 ETH". The first faulty reading was at 05:44:55 UTC, and the attack ran
within 30 seconds [5]. Reported bad debt was about $3.7M [6]. Moonwell zeroed supply and
borrow caps for wrsETH on Base and Optimism, and later moved those markets to
exchange-rate oracles [5]. The post-mortem does not explain why the feed returned that
value.

**Checks that would have fired.**

| Check | Why |
|---|---|
| `FW-JUMP-001` (experimental) | A change of about 1.65 million times between consecutive rounds. |
| `FW-CB-001` | Only if that feed had meaningful bounds. Not verified, so not claimed. |

`FW-DEV-001` would **not** have fired, because no exchange quotes wrsETH/ETH in USD; this
is the gap `FW-JUMP-001` is meant to cover. Detection would have taken up to one
`poll_interval`, while the attack took 30 seconds. Alerting alone is too slow against an
attacker who watches the same feed; this incident supports on-chain sanity bounds in the
consumer, with the monitor as a second line.

**Sources.**
5. Moonwell forum, "wrsETH Oracle Malfunction (11/4/25)": https://forum.moonwell.fi/t/wrseth-oracle-malfunction-11-4-25/2017
6. Web3 is Going Just Great, "Moonwell oracle malfunction" (links the CertiK alert): https://web3isgoinggreat.com/single/moonwell-oracle-malfunction

---

## 3. cbETH oracle misconfiguration: Moonwell (Base), 2026-02-15

**What happened.** A governance proposal (MIP-X43) enabled new oracle wrapper contracts.
For cbETH, the configuration "used only the raw cbETH/ETH exchange rate" without
multiplying by ETH/USD, so cbETH was priced at about $1.12 instead of about $2,200 [7].
Moonwell's monitoring detected the discrepancy about four minutes after execution, but
caps could only be lowered, and the fix had to go through a 5-day vote and timelock;
1,096.317 cbETH was liquidated and bad debt was about $1.78M [7][8].

**Relevance.** This was **not** a feed failure: every feed reported correctly. It is a
consumer integration error, the class of bug `FW-META-001` addresses in a narrower form
(units and decimals assumptions). `oraclewatch` v1 monitors raw feeds, so it **would not
have detected this**. It is listed to make that limitation explicit. Monitoring a
consumer's *derived* price (for example, reading the protocol's own oracle contract and
comparing it with `feed_a × feed_b` and a reference) is a future extension.

**Sources.**
7. Moonwell forum, "MIP-X43 cbETH Oracle Incident Summary": https://forum.moonwell.fi/t/mip-x43-cbeth-oracle-incident-summary/2068
8. The Block, "DeFi lending protocol Moonwell hit with $1.8 million bad debt after oracle misconfiguration": https://www.theblock.co/post/390302/defi-lending-protocol-moonwell-hit-with-1-8-million-bad-debt-after-oracle-misconfiguration

---

## 4. L2 sequencer outages: Arbitrum One, 2023-12-15 (risk condition, no confirmed oracle loss)

**What happened.** Arbitrum One's sequencer stalled at 10:29 ET during a surge of
inscription traffic. The batch poster stopped after 10 pending batches in the L1 mempool
[9]. Coverage puts the halt at about 78 minutes for the sequencer [10], with Dedaub
describing the network as "down for almost three hours" [9]. No source found ties this
outage to oracle-driven losses, so it is listed as a **risk condition**, not as an
exploit.

**Why it matters for oracle consumers.** While the sequencer is down, price feeds on the
L2 cannot update. When it returns, queued transactions run first, so the first blocks
can act on stale prices and liquidate users who could not top up during the outage. This
is why the official docs provide sequencer uptime feeds and a grace period (example:
one hour) [11], and why Aave v3 ships a `PriceOracleSentinel` that blocks borrows and
liquidations until the grace period after recovery has passed [12].

**Checks that would have fired.**

| Check | Why |
|---|---|
| `FW-SEQ-001` | Uptime feed `answer == 1` once the down status was recorded. |
| `FW-CHAIN-001` | Head timestamp of every Arbitrum RPC stops advancing. |
| `FW-SEQ-002` | For one hour after recovery. |
| `FW-STALE-001` | Suppressed while the chain is halted (reported as `FW-CHAIN-001`); fires on any feed that remains stale after recovery. |

**Sources.**
9. Dedaub, "Arbitrum Sequencer Outage: Root Cause Analysis": https://dedaub.com/blog/arbitrum-sequencer-outage/
10. The Block, "Arbitrum says all systems are operational after 'surge of inscriptions' caused outage earlier in day": https://www.theblock.co/post/267950/arbitrum-says-all-systems-are-operational-after-surge-of-inscriptions-caused-outage-earlier-in-day
11. Chainlink docs, L2 sequencer uptime feeds: https://docs.chain.link/data-feeds/l2-sequencer-feeds
12. Aave v3 docs, oracles and `PriceOracleSentinel`: https://aave.com/docs/aave-v3/smart-contracts/oracles

---

## 5. Recurring audit findings

These are findings from public audit contests, not incidents. They show which consumer
assumptions auditors repeatedly find missing, and they map one to one to checks.

| Finding (contest, severity) | Assumption missing on-chain | Check |
|---|---|---|
| "staleCheckLatestRoundData() doesn't check if a sequencer is down in Chainlink feeds" (Cyfrin CodeHawks, Foundry DeFi Stablecoin, Medium) [13] | Sequencer status | `FW-SEQ-001` |
| "ChainlinkUtils.getPrice consume data returned in invalid rounds" (Cyfrin CodeHawks, Zaros, Medium): sequencer `startedAt == 0` not rejected, prices used during the grace period [14] | Sequencer initialisation and grace period | `FW-INVALID-001`, `FW-SEQ-002` |
| Missing circuit-breaker (`minAnswer`/`maxAnswer`) handling: submitted repeatedly, e.g. The Standard, where judges ruled it a known issue [15] | Bounds awareness | `FW-CB-001` |

**Sources.**
13. https://codehawks.cyfrin.io/c/2023-07-foundry-defi-stablecoin/s/315
14. https://codehawks.cyfrin.io/c/2024-07-zaros/s/482
15. https://codehawks.cyfrin.io/c/2023-12-the-standard/s/444

---

## Considered and not included

- **Inverse Finance (June 2022):** caused by manipulable LP-token pricing built on top of
  correct feeds, not by a feed condition this tool observes.
- **USDC depeg (March 2023):** the feeds reflected the market; no source found showed an
  oracle failure. Used in `research.md` §6 only as evidence that stablecoin quotes can
  diverge by several percent.
