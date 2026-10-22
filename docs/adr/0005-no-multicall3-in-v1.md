# 5. No Multicall3 in v1

Date: 2026-10-22
Status: accepted

## Context

Multicall3 aggregates many `eth_call`s into one, reducing round trips and quota use. The
v1 feed set is 3 feeds and at most one sequencer feed per network, which is about 12
requests per minute per network with two endpoints (`docs/research.md` §7), well inside
every free tier.

## Decision

v1 sends one `eth_call` per read. Multicall3 is not used.

## Alternatives

- **Multicall3.** Saves requests, but: it adds a contract to trust per network, whose
  address must be verified like any other; one revert inside the batch has to be handled
  per call (`allowFailure`); and, more importantly, a single aggregated call returns one
  blob, so a disagreement between endpoints would show up as "the blob differs" instead
  of "this feed's answer differs".
- **JSON-RPC batch requests.** Fewer HTTP round trips without a contract, but providers
  limit and price batches differently, and the gain is small at this request rate.

## Consequences

- More requests per cycle than strictly necessary.
- Revisit when monitoring tens of feeds per network: Multicall3 with `allowFailure`, its
  address verified per network and recorded in the feed files, would then be the natural
  next step.
