<div align="center">
  <h1>uptop</h1>
  <p>Self-hosted uptime monitoring with a TUI over SSH.</p>
  <p>No browser. No client install. Just <code>ssh -p 23234 your-server</code>.</p>

  <p>
    <a href="https://gitea.lerkolabs.com/lerkolabs/uptop/actions/workflows/ci.yml"><img src="https://gitea.lerkolabs.com/lerkolabs/uptop/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT License">
    <img src="https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white" alt="Go 1.26">
    <img src="https://img.shields.io/docker/pulls/lerkolabs/uptop" alt="Docker Pulls">
  </p>

  <img src="assets/monitors.png" alt="uptop monitors view" width="800">
</div>

## What is this

An uptime monitor you manage entirely from the terminal. It runs as a server, exposes an SSH endpoint, and drops you into a full TUI — monitors, alerts, logs, nodes, all there.

Built on [RDGames/go-upkeep](https://github.com/RDGames/go-upkeep). Rewritten for clustering, config-as-code, and a proper dashboard.

## Features

- **6 check types** — HTTP, Push (heartbeat), Ping, Port, DNS, Groups
- **10 alert providers** — Discord, Slack, Email, Ntfy, Webhook, Telegram, PagerDuty, Pushover, Gotify, Opsgenie
- **Config as code** — define monitors in YAML, apply declaratively, version control your setup
- **HA clustering** — leader/follower with automatic failover
- **Prometheus metrics** — `/metrics` endpoint, wire it straight to Grafana
- **Public status page** — HTML + JSON, toggle with an env var
- **SQLite or Postgres** — SQLite for single-node, Postgres for production
- **Uptime Kuma import** — migrate from Kuma with one command

## Screenshots

<table>
  <tr>
    <td><img src="assets/detail.png" alt="detail panel" width="400"></td>
    <td><img src="assets/alerts.png" alt="alerts view" width="400"></td>
  </tr>
  <tr>
    <td><img src="assets/logs.png" alt="logs view" width="400"></td>
    <td><img src="assets/nodes.png" alt="cluster nodes" width="400"></td>
  </tr>
  <tr>
    <td colspan="2" align="center"><img src="assets/theme.png" alt="theme selection" width="600"></td>
  </tr>
</table>

## Quick start

```bash
go run cmd/uptop/main.go
ssh -p 23234 localhost
```

Want some data to look at first:

```bash
go run cmd/uptop/main.go -demo
```

## Install

<details>
<summary><strong>Docker (recommended)</strong></summary>

```yaml
services:
  uptop:
    image: lerkolabs/uptop:latest
    restart: unless-stopped
    ports:
      - "23234:23234"
      - "8080:8080"
    environment:
      - UPTOP_DB_TYPE=sqlite
      - UPTOP_DB_DSN=/data/uptop.db
      - UPTOP_STATUS_ENABLED=true
      # - UPTOP_ADMIN_KEY=ssh-ed25519 AAAA... you@host
    volumes:
      - ./data:/data
```

First run: set `UPTOP_ADMIN_KEY` to your SSH public key, or attach to the container and add it in the Users tab.

</details>

<details>
<summary><strong>Binary (Linux amd64)</strong></summary>

Download from [Releases](https://github.com/lerkolabs/uptop/releases).

</details>

<details>
<summary><strong>From source</strong></summary>

```bash
go install gitea.lerkolabs.com/lerkolabs/uptop/cmd/uptop@latest
```

</details>

**Upgrading:** Pull the new image (or binary) and restart. Database migrations run automatically on startup.

## Config as code

Export your current monitors:

```bash
uptop export -o monitors.yaml
```

Apply a config file:

```bash
uptop apply -f monitors.yaml
uptop apply -f monitors.yaml --dry-run   # see what would change
uptop apply -f monitors.yaml --prune     # delete anything not in the YAML
```

Full reference in [docs/config-as-code.md](docs/config-as-code.md).

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `UPTOP_PORT` | `23234` | SSH server port |
| `UPTOP_HTTP_PORT` | `8080` | HTTP server port (status page, push, metrics) |
| `UPTOP_DB_TYPE` | `sqlite` | `sqlite` or `postgres` |
| `UPTOP_DB_DSN` | `uptop.db` | Database path or connection string |
| `UPTOP_STATUS_ENABLED` | `false` | Enable public status page |
| `UPTOP_STATUS_TITLE` | `System Status` | Status page title |
| `UPTOP_ENCRYPTION_KEY` | | AES-256-GCM key for alert credentials ([details](#encryption)) |
| `UPTOP_CLUSTER_MODE` | `leader` | `leader`, `follower`, or `probe` |
| `UPTOP_PEER_URL` | | Leader URL for follower and probe nodes |
| `UPTOP_CLUSTER_SECRET` | | Shared key for cluster + API auth |
| `UPTOP_INSECURE_SKIP_VERIFY` | `false` | Skip TLS verification for checks |
| `UPTOP_ALLOW_PRIVATE_TARGETS` | `false` | Allow monitoring RFC1918/loopback addresses |
| `UPTOP_ADMIN_KEY` | | SSH public key seeded as first admin on startup |
| `UPTOP_TRUSTED_PROXIES` | | Comma-separated CIDRs/IPs whose `X-Forwarded-For` is trusted ([details](#running-behind-a-reverse-proxy)) |

See [`.env.example`](.env.example) for all options including TLS, probes, and advanced settings.

### Running behind a reverse proxy

By default uptop ignores the `X-Forwarded-For` header and rate-limits by the direct connection address — so a client can't spoof the header to bypass limits. If uptop sits behind a reverse proxy (nginx, Caddy, Cloudflare, an ALB), set `UPTOP_TRUSTED_PROXIES` to the proxy's address(es) so the real client IP is used instead:

    # single nginx/Caddy on the same host
    UPTOP_TRUSTED_PROXIES=127.0.0.1

    # a proxy subnet, or Cloudflare ranges
    UPTOP_TRUSTED_PROXIES=10.0.0.0/8,172.16.0.0/12

Only requests whose immediate peer is in this list have their `X-Forwarded-For` honored (right-most non-trusted hop wins). Bare IPs are treated as single hosts; invalid entries are warned about and skipped. Leave it unset if uptop is exposed directly.

### Encryption

Set `UPTOP_ENCRYPTION_KEY` to encrypt alert credentials (SMTP passwords, webhook URLs, API tokens) at rest with AES-256-GCM. Generate a key:

    openssl rand -hex 32

Without this, credentials are stored as plaintext in the database. uptop warns on startup if unset. To encrypt credentials on an existing install, run `uptop migrate-secrets` with the key set.

## Clustering

uptop supports three modes: **leader** (default single node), **follower** (HA failover — takes over if the leader goes down), and **probe** (stateless distributed checks from multiple regions).

See [docs/clustering.md](docs/clustering.md) for setup guides, or the working examples in [`deploy/`](deploy/).

## Migrating from Uptime Kuma

Export your Kuma backup JSON, then:

```bash
curl -X POST http://localhost:8080/api/import/kuma \
  -H "X-Uptop-Secret: your-secret" \
  -H "Content-Type: application/json" \
  -d @kuma-backup.json
```

## License

MIT — see [LICENSE](LICENSE).
