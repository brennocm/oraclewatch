# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - unreleased

### Added

- Research notes, verified feed lists (Ethereum, Arbitrum, Base, OP) and incident catalog.
- 16 checks: staleness, invalid rounds, deviation from a market reference, reference
  degradation, circuit-breaker proximity, aggregator and metadata changes, round jumps
  (experimental), RPC disagreement/fork/wrong chain, chain stall, L2 sequencer down and
  grace period, monitor blindness, cross-chain divergence.
- Pinned-block reads across several RPC endpoints with retries, circuit breaker and chain
  ID verification.
- Market reference from Coinbase, Kraken, Bitstamp and Gemini (median, spread, untrusted state).
- Alert state machine with persistence, hysteresis, cooldown, escalation and storm
  grouping; Discord, Telegram and stdout notifiers.
- `run`, `check`, `validate-config [--online]`, `feeds list`, `healthcheck`, `version`.
- Prometheus metrics, `/healthz`, `/readyz`, `/status`; provisioned Grafana dashboard.
- Hardened docker compose stack and Oracle Cloud Always Free guide.
- Signed multi-arch release images with SBOM and build provenance.

### Security

- Secrets are redacted from logs, errors, metrics, `/status` and alerts (end-to-end test).
- JSON-RPC response bodies are capped at 4 MiB.
- `gorilla/websocket` bumped to v1.5.3 (GO-2026-6278).
