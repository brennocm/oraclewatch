# Deployment

Two ways to run oraclewatch:

1. **Binary or container only.** `oraclewatch run --config config.yaml` with metrics on
   `127.0.0.1:9090`. Good for an existing Prometheus/Grafana stack.
2. **The bundled stack** (`deploy/docker-compose.yml`): oraclewatch, Prometheus, Grafana
   with the provisioned dashboard, and Caddy for automatic TLS. This guide targets an
   Oracle Cloud Always Free Ampere A1 instance (arm64), which is why the image is
   multi-arch.

Nothing here deploys anything by itself. You run every step.

## 1. The stack

```
            internet
               │ 80/443 (only published ports)
            ┌──▼───┐   edge network
            │ Caddy│──────────────┐
            └──────┘              │
                              ┌───▼────┐   monitoring network (internal: no internet)
                              │Grafana │◄────────────┐
                              └────────┘             │
                                              ┌──────┴─────┐ scrape ┌─────────────┐
                                              │ Prometheus │───────►│ oraclewatch │──► RPCs, exchanges,
                                              └────────────┘        └─────────────┘    Discord, Telegram
                                                                     egress network
```

Every container is `read_only`, drops all capabilities (Caddy adds back only
`NET_BIND_SERVICE`), runs with `no-new-privileges`, has memory, CPU and PID limits, and
uses an image pinned by digest. Prometheus and oraclewatch's HTTP endpoints are never
published. Grafana has sign-up disabled and its admin password comes from `.env`;
anonymous read-only access is off unless you set `GRAFANA_ANONYMOUS=true`.

## 2. Oracle Cloud Always Free

Limits checked on 2026-10-22 in the Oracle docs
(<https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm>):

- Ampere A1 (`VM.Standard.A1.Flex`): 1,500 OCPU-hours and 9,000 GB-hours per month for
  all tenancies, which for Always Free tenancies is **2 OCPUs and 12 GB of memory** in
  total. The stack needs well under 1 OCPU and 1 GB: use **1 OCPU / 6 GB** and keep the
  rest for other workloads.
- Block storage: 200 GB in total (boot + block volumes) in the home region. The default
  50 GB boot volume is enough.
- Outbound data transfer: 10 TB per month.
- **Idle reclamation:** Oracle may reclaim Always Free instances that, over 7 days, stay
  below 20% at the 95th percentile on CPU, network **and** memory (memory applies to A1
  shapes). A monitoring stack is light, so check this policy for your account type before
  relying on the instance; otherwise your monitor can disappear without warning. The
  external dead man's switch (`self_health.deadman_url`) is what tells you if it does.

### 2.1 Network

In the VCN's security list, or better a network security group attached to the instance:

| Direction | Protocol | Port | Source | Why |
|---|---|---|---|---|
| Ingress | TCP | 22 | **your public IP /32 only** | SSH, key-only |
| Ingress | TCP | 80 | 0.0.0.0/0 | ACME HTTP-01 challenge and redirect |
| Ingress | TCP | 443 | 0.0.0.0/0 | Grafana over HTTPS |
| Ingress | UDP | 443 | 0.0.0.0/0 | HTTP/3 (optional) |
| Egress | all | all | 0.0.0.0/0 | RPCs, exchanges, notifiers, ACME, updates |

Remove the default "SSH from 0.0.0.0/0" rule.

### 2.2 Instance

- Image: Canonical Ubuntu 24.04 (aarch64). Shape: `VM.Standard.A1.Flex`, 1 OCPU, 6 GB.
- SSH key: your own public key; never generate one in the console and download it.
- Cloud-init: paste `deploy/oci/cloud-init.yaml` under "Advanced options → Management".

It installs Docker and unattended upgrades, disables SSH passwords and root login, opens
80/443 in the host firewall (the OCI image rejects everything but SSH by default), and
creates an unprivileged `oraclewatch` service account that owns `/opt/oraclewatch`.

### 2.3 DNS and TLS

Point an A record at the instance's public IP. Without a domain, a free dynamic-DNS
subdomain (for example DuckDNS) works with Let's Encrypt. Caddy obtains and renews the
certificate on first start; ports 80 and 443 must be reachable for that.

## 3. Install

On the instance, as the default user:

```sh
sudo -u oraclewatch git clone --depth 1 --branch v0.1.0 https://github.com/brennocm/oraclewatch /opt/oraclewatch/src
cd /opt/oraclewatch/src
```

Copy your `.env` from your machine (never paste secrets into a shell history on the
server):

```sh
scp .env ubuntu@<ip>:/tmp/oraclewatch.env
ssh ubuntu@<ip> 'sudo install -o oraclewatch -g oraclewatch -m 0600 /tmp/oraclewatch.env /opt/oraclewatch/src/.env && rm /tmp/oraclewatch.env'
```

Required in `.env`: the RPC URLs (two providers per network), `GRAFANA_ADMIN_PASSWORD`
(long and random), `GRAFANA_DOMAIN`, `ACME_EMAIL`. Optional: Discord, Telegram,
`DEADMAN_URL`.

Validate the configuration on-chain before starting:

```sh
sudo -u oraclewatch docker run --rm --env-file .env -v "$PWD/configs/example.yaml:/c.yaml:ro" \
  ghcr.io/brennocm/oraclewatch:v0.1.0 validate-config --config /c.yaml --online
```

Verify and pin the image (section 4), then start:

```sh
sudo -u oraclewatch docker compose -f deploy/docker-compose.yml --env-file .env up -d
sudo -u oraclewatch docker compose -f deploy/docker-compose.yml ps
```

`https://<GRAFANA_DOMAIN>` should show the dashboard after the first cycle.

## 4. Verifying and updating the image

Release images are signed keylessly with cosign from the release workflow and carry a
SLSA build provenance attestation. Verify before every update:

```sh
cosign verify ghcr.io/brennocm/oraclewatch:v0.1.0 \
  --certificate-identity-regexp '^https://github.com/brennocm/oraclewatch/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

gh attestation verify oci://ghcr.io/brennocm/oraclewatch:v0.1.0 --owner brennocm
```

Then pin the verified digest in `.env`, so that a re-pushed tag cannot change what runs:

```sh
ORACLEWATCH_IMAGE=ghcr.io/brennocm/oraclewatch@sha256:<digest printed by cosign>
```

To update: `git -C /opt/oraclewatch/src fetch --tags && git checkout <new tag>`, verify
the new image, update the digest, and run `docker compose ... up -d`.

## 5. Backup and recovery

Configuration and dashboards are in git; secrets are in your local `.env`. Monitoring
state is in memory by design (ADR 0007) and Prometheus history is disposable. Recovering
from a lost instance is: create a new instance with the same cloud-init, copy `.env`,
start the stack.

## 6. Operating notes

- Rate limiting in front of Grafana needs a Caddy plugin (for example `caddy-ratelimit`)
  built into a custom image; it is not included. Grafana's own login attempt limits apply.
- Exposing oraclewatch's `/status` or Prometheus publicly is the operator's decision and
  not supported by this compose file. They contain no secrets, but they reveal what you
  monitor and when it is unhealthy.
- Logs: `docker compose logs oraclewatch`. Secrets are redacted before they are written
  (`internal/redact`), including errors from the HTTP stack.
