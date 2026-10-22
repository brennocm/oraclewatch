# 1. Record architecture decisions

Date: 2026-10-22
Status: accepted

## Context

oraclewatch makes several decisions whose reasons are not visible in the code: which
numeric types decide alerts, how RPC endpoints are trusted, what is read at which block,
what is deliberately left out. Reviewers and future maintainers need to see the
reasoning and the alternatives that were rejected, not only the result.

## Decision

Significant decisions are recorded as Architecture Decision Records in `docs/adr/`,
numbered sequentially (`NNNN-short-title.md`), with the sections Context, Decision,
Alternatives, Consequences. A record is never rewritten after it is accepted; a later
record supersedes it and both link to each other.

## Alternatives

- Comments in code only: they explain how, rarely why, and cannot hold alternatives.
- A single design document: it goes stale and hides the order in which decisions were
  made.

## Consequences

Each decision that changes behaviour, trust assumptions or dependencies comes with an
ADR in the same commit. This costs a few minutes per decision and makes reviews and
interviews about the design straightforward.
