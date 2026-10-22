# FW-SELF-001: Monitor blind

- **Maximum severity:** critical
- **Status:** stable
- **Implementation:** `internal/checks/`

## What it checks

Critical when no feed of the network could be read for `blind_after_cycles` consecutive cycles; warning when no reference source answered for that long. Optional external dead man's switch: `self_health.deadman_url` is pinged after each successful cycle, so a dead process is noticed from outside.

## Why it matters

The worst failure of a monitor is a silent one.

## Related incident

—

## Parameters

`self_health.blind_after_cycles` (3), `self_health.deadman_url`.

## Known false positives and tuning

None; if it fires, every other check for the network is Unknown.
