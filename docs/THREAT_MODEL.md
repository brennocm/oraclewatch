# Threat model

Scope: the oraclewatch process, its configuration and secrets, its inputs (RPC endpoints,
exchange APIs), its outputs (alerts, metrics, HTTP endpoints), the release pipeline, and the
reference deployment (`deploy/`, Oracle Cloud VM). Out of scope: the security of Chainlink's
contracts and infrastructure, and of the protocols that consume the feeds.

## Assets

| Asset | Why it matters |
|---|---|
| **Integrity of alerts** | A missed alert (false negative) is the worst outcome: the operator believes the feed is healthy while their protocol is exposed. A flood of false alerts is the second worst: it trains people to ignore the channel. |
| **Availability of the monitor** | A monitor that is silently down is a false negative for every check. |
| RPC API keys | Usually embedded in RPC URLs; leaking them costs quota or money and may reveal what is monitored. |
| Discord webhook URL, Telegram bot token | Anyone holding them can post to the alert channel, for example to forge "resolved" messages. |
| The VM | Holds the secrets and the stack. |
| Release artifacts | Users run the image with their secrets. |

oraclewatch holds **no private keys and cannot send transactions**: there is nothing on the
host that moves funds.

## Adversaries and threats

| # | Threat | Mitigations | Residual risk |
|---|---|---|---|
| T1 | **Malicious or compromised RPC lies** (fake answer, fake timestamp, old state) to hide a problem or cause a false alarm. | Several endpoints per network; all reads at one pinned block; block hashes compared (`FW-RPC-002`) and feed tuples compared byte for byte (`FW-RPC-001`, critical on any difference); impossible values rejected (`FW-INVALID-001`: `updatedAt` after the block, round going backwards); chain ID verified (`FW-RPC-003`), and an endpoint is never read before its chain ID has matched once (an endpoint whose first check failed transiently is re-checked every cycle and excluded until then). | With two endpoints that disagree, oraclewatch cannot tell which is honest: it alerts critical and keeps reading the configuration-order winner. Endpoints from the same provider (or colluding providers) can lie consistently. Mitigation: use independent providers, a third endpoint, or your own node. Only `latestRoundData` is compared across endpoints; metadata, `decimals()` and bounds are read from the first endpoint that answers, so a lying first endpoint could hide an aggregator change or report wrong bounds (comparing them too is listed as future work). |
| T2 | **RPC lags or is on a fork**, so the feed looks fresh or stale wrongly. | Heads older than `max_head_lag` excluded and reported (`FW-CHAIN-001`); staleness measured against the pinned block's time; all-lagging → stalled chain, feed checks report Unknown instead of N false alarms. | A lagging-but-within-limit endpoint drags the pinned block back by up to `max_head_lag`. |
| T3 | **Exchange returns a wrong or manipulated price**, or an attacker on the path tampers with it. | HTTPS; median of several sources; untrusted state when sources disagree beyond `max_source_spread` or too few answer (`FW-REF-001`), in which case `FW-DEV-001` does not fire; strict parsing (decimal strings only, crossed books rejected, object-only decoding, 64 KiB limit); stale quotes dropped. | Several exchanges moving together (thin market, real depeg) produce a real deviation alert, which is intended. Kraken and Gemini provide no quote timestamp. |
| T4 | **Alert fatigue / flooding**, by an attacker who can induce failures (for example making one RPC flap) or by a real outage. | `for:` persistence, hysteresis on resolve, cooldown with escalation-only bypass, grouping window and storm collapse into one message; Unknown never fires. | A long real outage still produces one storm message per window. |
| T5 | **Monitor blind without anyone noticing** (crash, VM reclaimed, all RPCs down, network partition). | `FW-SELF-001` (critical after `blind_after_cycles`); `/readyz`; optional external dead man's switch that stops being pinged when any network is unhealthy or the process is gone; container healthcheck and restart policy. `check` mode fails immediately when blind. | Without an external dead man's switch, a dead host cannot report itself. Configure one. |
| T6 | **Secret leakage** through logs, metrics, `/status`, alerts or errors (the stdlib embeds request URLs in `*url.Error`). | Secrets only enter through `config.Secret` fields from the environment; `Secret` never prints its value; registered with the redactor at startup (including URL path segments and query values); `*url.Error` always rewritten even if unregistered; `slog` handler redacts every attribute; metric labels are configured names only; end-to-end test injects known secrets and checks every output, and was mutation-tested. | A secret shorter than 6 characters is not redacted (by design, to avoid mangling output); provider keys are far longer. |
| T7 | **Forged alerts** via a leaked webhook/token. | Same as T6; tokens never logged; Discord mentions disabled in payloads. | Discord webhooks cannot be restricted to one sender; rotate on suspicion. |
| T7b | **Injection through on-chain text**: a malicious contract, or a lying RPC, returns a `description()`/`typeAndVersion()` containing markdown links, mentions or terminal escape sequences, which then appears inside alerts (phishing in the alert channel) or on a terminal. | Contract strings are reduced to printable ASCII and capped at 128 bytes when read (`feeds.SanitizeText`); Discord descriptions and field values are markdown-escaped and `allowed_mentions` is empty; Telegram is sent as plain text; JSON outputs are encoded. | Plain URLs in Telegram may still be auto-linked by clients (previews are disabled). |
| T8 | **Compromise of the VM.** | oraclewatch receives only the environment variables it needs (not the Grafana admin password); containers read-only, all capabilities dropped, `no-new-privileges`, non-root distroless image with no shell, resource limits; SSH key-only, no root login, restricted to the operator's IP; only 80/443 public; Prometheus and oraclewatch never published; unattended upgrades. | The `oraclewatch` service account is in the `docker` group, which is root-equivalent on the host. Rootless Docker would remove that at the cost of complexity. Secrets on the host are readable by root. |
| T9 | **Denial of service through crafted responses** (huge or slow bodies, malformed ABI data). | Per-call timeouts, transport timeouts, bounded retries; 4 MiB limit on JSON-RPC response bodies (the go-ethereum client has none, so oraclewatch wraps its transport), 64 KiB for exchanges, 16 KiB read for notifier responses, 1 MiB config limit; ABI decoders fuzzed; one slow network cannot delay others (separate workers) and overrunning cycles drop ticks. | A response just under the limit still costs a few MiB per call. |
| T10 | **Supply chain**: compromised dependency, action or base image. | Few dependencies, each justified; `govulncheck` in CI; Dependabot for Go, Actions and Docker; actions pinned by commit SHA; base images pinned by digest; release images signed (cosign keyless) with SBOM and build provenance; a test forbids key-management packages in the binary. | Trust in go-ethereum and the Go toolchain remains. |
| T11 | **Misconfiguration** (wrong address, chain, decimals, threshold). | Addresses only from official data files, verified on-chain and checked by a test against the example; EIP-55 checksum required; `validate-config --online`; `FW-META-001` and `FW-RPC-003` at runtime; deviation alert level forced to at least twice the feed's threshold. | Heartbeats and thresholds cannot be verified on-chain; a wrong heartbeat produces wrong staleness alerts. |

## Accepted residual risks

- **Detection latency** of one poll interval plus confirmations. Attacks that exploit a bad
  price within a block cannot be stopped by an off-chain monitor; on-chain checks in the
  consumer are the first line, as the README states.
- **In-memory state** (ADR 0007): a restart re-alerts persisting conditions and forgets
  cleared ones. Chosen over a writable volume in a read-only container.
- **Public RPC and exchange APIs** have no availability guarantees. The design degrades to
  Unknown plus `FW-SELF-001`/`FW-REF-001` instead of guessing.
- **Docker group membership** of the service account (T8), documented in the deployment
  guide.
