# Security policy

## Supported versions

Only the latest release receives security fixes.

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub private vulnerability reporting](https://github.com/brennocm/oraclewatch/security/advisories/new).
Do not open a public issue.

Include the affected version, a description of the issue, and steps to reproduce. You
should receive an acknowledgement within 72 hours and a status update within 7 days.

## Scope

In scope:

- Leaks of secrets (RPC URLs with API keys, webhook URLs, bot tokens) through logs,
  metrics, the HTTP endpoints, alerts or error messages.
- Conditions in which a real oracle problem would not produce an alert (false
  negatives), including those caused by a malicious RPC endpoint or price source.
- Denial of service of the monitor through crafted responses from RPCs, exchanges or
  notifier endpoints.
- Weaknesses in the published container image, release artifacts or their signatures.

Out of scope:

- Vulnerabilities in Chainlink contracts or infrastructure. Report those to Chainlink
  through its own channels.
- Issues that require an attacker who already controls the host running oraclewatch.

oraclewatch never sends transactions and holds no private keys; a report that it can be
made to do either would be treated as critical.

The threat model is in [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md).
