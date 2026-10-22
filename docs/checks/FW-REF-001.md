# FW-REF-001: Reference degraded

- **Maximum severity:** warning
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

Warning when fewer than `reference.min_sources` sources returned a valid quote, or `(max - min) / median > reference.max_source_spread`. While it fires, FW-DEV-001 reports Unknown.

## Why it matters

One broken or manipulated exchange must neither fire nor hide a deviation alert. This makes the blind spot visible instead of silent.

## Related incident

—

## Parameters

`reference.min_sources` (2), `reference.max_source_spread` (1%), `reference.max_quote_age` (2m).

## Known false positives and tuning

Regional API blocks and rate limits make this fire; add a third source. During a real stablecoin depeg exchanges can disagree by more than 1%, in which case the reference is honestly untrusted.
