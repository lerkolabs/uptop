# Config as Code

Define your monitors and alerts in a YAML file. Version control them, copy them between instances, or spin up a fresh setup in one command.

## Quick start

Export what you already have:

```bash
uptop export -o monitors.yaml
```

That gives you a working file you can edit and re-apply:

```bash
uptop apply -f monitors.yaml
```

That's it. Apply only creates or updates — it won't delete anything unless you tell it to.

## The YAML file

Two top-level sections: `alerts` and `monitors`. Alerts go first because monitors reference them by name.

```yaml
alerts:
  - name: Discord Ops
    type: discord
    settings:
      url: https://discord.com/api/webhooks/your/token

  - name: PagerDuty Critical
    type: pagerduty
    settings:
      routing_key: your-integration-key
      severity: critical

monitors:
  - name: API
    type: http
    url: https://api.example.com/health
    interval: 30
    alert: Discord Ops

  - name: Production
    type: group
    alert: PagerDuty Critical
    monitors:
      - name: Prod Web
        type: http
        url: https://prod.example.com
        interval: 15
      - name: Prod DB
        type: port
        hostname: db.internal
        port: 5432
        interval: 30
```

## Monitor types

Each type has required fields. Everything else is optional with sensible defaults.

**http** — polls a URL
```yaml
- name: My API
  type: http
  url: https://api.example.com/health
  interval: 30
```

Optional: `method` (default GET), `accepted_codes` (default 200-299), `timeout`, `check_ssl`, `expiry_threshold` (default 7 days), `max_retries`, `ignore_tls`, `description`, `paused`.

**ping** — ICMP ping a host
```yaml
- name: Gateway
  type: ping
  hostname: 10.0.0.1
  interval: 30
```

**port** — check if a port is open
```yaml
- name: SSH Server
  type: port
  hostname: 10.0.0.1
  port: 22
  interval: 60
```

**dns** — resolve a hostname
```yaml
- name: DNS Check
  type: dns
  hostname: example.com
  dns_resolve_type: A
  dns_server: 1.1.1.1
  interval: 60
```

**push** — heartbeat endpoint for cron jobs
```yaml
- name: Nightly Backup
  type: push
  interval: 86400
```

Push monitors get a token assigned automatically. Hit the push endpoint before the interval expires or it alerts.

**group** — organize monitors together
```yaml
- name: Production
  type: group
  monitors:
    - name: Web
      type: http
      url: https://prod.example.com
      interval: 15
```

Groups can't nest inside other groups. A group is healthy when all its children are healthy.

## Alert types

All 10 providers work in the YAML. The `settings` map is different per type.

```yaml
# Discord / Slack / Generic Webhook — just a URL
- name: Discord Ops
  type: discord
  settings:
    url: https://discord.com/api/webhooks/your/token

# Email
- name: Email Oncall
  type: email
  settings:
    host: smtp.example.com
    port: "587"
    user: oncall@example.com
    pass: your-password
    from: oncall@example.com
    to: team@example.com

# Ntfy
- name: Ntfy Alerts
  type: ntfy
  settings:
    url: https://ntfy.sh
    topic: my-alerts
    priority: "4"
    # for protected topics:
    # username: user
    # password: pass

# Telegram
- name: Telegram Ops
  type: telegram
  settings:
    token: "123456:ABC-DEF..."
    chat_id: "-1001234567890"

# PagerDuty
- name: PD Critical
  type: pagerduty
  settings:
    routing_key: your-integration-key
    severity: critical

# Pushover
- name: Pushover
  type: pushover
  settings:
    token: app-token
    user: user-key

# Gotify
- name: Gotify
  type: gotify
  settings:
    url: https://gotify.example.com
    token: app-token
    priority: "8"

# Opsgenie
- name: Opsgenie
  type: opsgenie
  settings:
    api_key: your-api-key
    priority: P2        # P1–P5, default P3
    # eu: "true"        # use the EU API endpoint
```

## Commands

**Export current state:**
```bash
uptop export -o monitors.yaml      # to a file
uptop export                        # to stdout
```

**Apply a config:**
```bash
uptop apply -f monitors.yaml
```

**See what would change first:**
```bash
uptop apply -f monitors.yaml --dry-run
```

**Delete monitors not in the YAML:**
```bash
uptop apply -f monitors.yaml --prune
```

Without `--prune`, apply never deletes anything. It only creates and updates.

**Pointing at a different database:**
```bash
uptop export -db-type postgres -dsn "host=localhost dbname=uptop sslmode=disable"
uptop apply -f monitors.yaml -db-type postgres -dsn "..."
```

Both commands respect the `UPTOP_DB_TYPE` and `UPTOP_DB_DSN` environment variables too.

## How apply works

Monitors and alerts are matched by **name**. Names must be unique across the entire file.

1. Alerts are resolved first (created or updated)
2. Groups are created next (so children can reference them)
3. Everything else is created or updated
4. If `--prune` is set, anything in the database that's not in the YAML gets deleted

Apply is idempotent. Run it twice with the same file, second run changes nothing.

Apply is **not atomic** — items are written one at a time, so an error mid-apply (bad value, lost DB connection, ctrl-C) leaves the items already written in place. That's safe to recover from: apply diffs against the database by name, so fix the issue and run it again — it converges the rest. Just don't run two applies against the same database at once.

## Backups and secrets

`uptop export` writes alert credentials (SMTP passwords, API tokens, webhook URLs) into the YAML in clear text — that's what makes the file restorable. Treat it like a secrets file.

The HTTP export endpoint redacts those same fields **by default**:

```bash
# secrets show as ***REDACTED*** — fine for sharing or review
curl -H "X-Uptop-Secret: your-secret" \
  "http://localhost:8080/api/backup/export"

# full backup you can actually restore from
curl -H "X-Uptop-Secret: your-secret" \
  "http://localhost:8080/api/backup/export?redact_secrets=false"
```

Restoring a redacted export imports the literal string `***REDACTED***` as your credentials. For real backups, pass `redact_secrets=false` or run `uptop export` on the host.

## Typical workflow

```bash
# set up your monitors in the TUI first, then export
uptop export -o monitors.yaml

# commit it
git add monitors.yaml && git commit -m "add monitor config"

# deploy to another instance
scp monitors.yaml prod-server:
ssh prod-server uptop apply -f monitors.yaml

# or just keep it as a backup you can restore from
uptop apply -f monitors.yaml
```
