# FW-META-001: Metadata drift

- **Maximum severity:** critical
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

Critical if `decimals()` differs from `expected_decimals`; warning if `description()` differs from `expected_description`; info if `version()` changed since the previous read.

## Why it matters

Consumers that hard-code decimals misprice by powers of ten when they change.

## Related incident

Moonwell cbETH, Feb 2026 (related class: unit/composition errors).

## Parameters

`expected_decimals`, `expected_description` per feed.

## Known false positives and tuning

After a deliberate migration, update the config from a re-verified `configs/feeds/*.yaml`.
