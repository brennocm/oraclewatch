# FW-DEV-001: Deviation from market reference

- **Maximum severity:** critical
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

`dev = |oracle - reference| / reference` with `reference` the median of trusted exchange mids (FW-REF-001). Warning above `warn_above`, critical above `critical_above`; equality does not fire.

## Why it matters

Detects a feed frozen or wrong while the market moves. Validation enforces `warn_above >= 2 × deviation_threshold`, since a healthy feed can lag the market by up to its own threshold until the next round.

## Related incident

LUNA/USD, May 2022 (oracle $0.107 vs market about $0.01).

## Parameters

`deviation.warn_above` (2%), `deviation.critical_above` (5%), alert `for:` (2m) for persistence.

## Known false positives and tuning

Fast markets: the pinned block is a few seconds old and exchange quotes are current, so a violent move can show a temporary gap; `for:` absorbs it. SVR feeds may add a configurable delay. Stablecoin depegs are real deviations, not false positives.
