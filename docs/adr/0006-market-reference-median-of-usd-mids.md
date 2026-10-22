# 6. Market reference: median of USD mid prices, with an explicit "untrusted" state

Date: 2026-10-22
Status: accepted

## Context

`FW-DEV-001` compares a feed with the market. The market reading is itself an untrusted
input: an exchange can be down, rate limit us, return a stale or manipulated price, or
change its API. A deviation alert based on a bad reference is a false positive at best;
a reference that silently follows a broken exchange can also hide a real deviation.

## Decision

- Sources: Coinbase Exchange, Kraken, Bitstamp, Gemini public tickers (no keys), as
  researched in `docs/research.md` §6. Only USD-quoted markets; USDT/USDC quotes are
  rejected by config validation.
- Price per source: **mid = (bid + ask) / 2**, parsed from strings into `big.Rat`. Crossed
  books, non-positive values and non-decimal formats are rejected.
- Combination: **median** of the accepted quotes. With three or more sources one wrong
  source cannot move it.
- **Untrusted** when fewer than `min_sources` answered, or when
  `(max - min) / median > max_source_spread`. An untrusted reference never fires
  `FW-DEV-001`; it raises `FW-REF-001` instead.
- Quotes with an exchange timestamp older than `max_quote_age`, or more than one minute
  in the future, are dropped.
- Each source/pair result (including errors) is cached for `cache_ttl` so that four
  networks asking for ETH-USD in the same minute cost one request per exchange. A 429
  pauses the source for `Retry-After` (default 60 s).
- Responses are limited to 64 KiB and decoded strictly; Bitstamp's habit of answering an
  unknown market with HTTP 200 and an array of every ticker is rejected by decoding into
  an object.

## Alternatives

- **Mean.** One manipulated source shifts it proportionally.
- **Last trade price.** Can be minutes old on a quiet market; the mid reflects the book now.
- **Aggregators (CoinGecko, CoinMarketCap).** One more opaque layer, API keys, and they
  partly derive from the same exchanges.
- **Ignore spread, trust the median.** With two sources the median is the mean, so a single
  bad source would move it; the spread check is what catches that case.

## Consequences

- With the default `min_sources: 2`, two sources that both fail make the deviation check
  go quiet for that pair; this is visible as `FW-REF-001` and in metrics, never silent.
- Kraken and Gemini provide no usable quote timestamp, so their staleness relies on the
  fetch time; documented as a limitation.
