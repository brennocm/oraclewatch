# 3. Exact arithmetic with math/big for every alert decision

Date: 2026-10-22
Status: accepted

## Context

Feed answers are `int256` fixed-point integers with `decimals()` digits (8 for the USD
feeds monitored here, 18 on some others). Alert decisions compare ratios of those values
with thresholds expressed as percentages: deviation from a reference (`|a - r| / r > 2%`),
distance to `minAnswer`/`maxAnswer`, cross-chain divergence, round-to-round jumps.

`float64` has 53 bits of mantissa. It cannot represent many of the thresholds exactly
(0.1% and 0.15% are repeating fractions in binary), and it silently loses precision on
large `int256` values: `maxAnswer` on the ETH/USD aggregators is about 9.6e52, far above
2^53. Comparisons near a threshold, which are exactly the comparisons that decide whether
an alert fires, can then go either way depending on rounding. Reviewers of an oracle
monitor will check this first.

## Decision

- Raw on-chain values are `*big.Int` from the ABI decoder to the checks.
- Normalised prices, ratios and thresholds are `*big.Rat`. Percentages in the config are
  parsed from their decimal text straight into `big.Rat` (`config.Percent`): "0.5%" is
  exactly 1/200.
- Exchange prices are parsed from the JSON string into `big.Rat`; JSON numbers are never
  decoded into `float64`.
- Comparisons use `Cmp`. No check converts to `float64`.
- `float64` is allowed only when exporting Prometheus gauges, which are for display.

## Alternatives

- **float64 with an epsilon.** Moves the problem to choosing an epsilon and still loses
  precision on large values.
- **Fixed-point integers with a common scale (e.g. 1e18).** Exact, but every operation
  needs manual rescaling and division rounding rules; `big.Rat` expresses the same maths
  directly and readably.
- **shopspring/decimal or similar.** An extra dependency for what the standard library
  already provides.

## Consequences

- Allocation-heavy compared with float maths. At tens of feeds per minute this is
  negligible.
- Tests can assert exact boundaries ("exactly at the threshold does not fire, one unit of
  the last decimal above does") without tolerances.
- Code that formats values for humans must do so explicitly (`FloatString`), which keeps
  display and decision paths visibly separate.
