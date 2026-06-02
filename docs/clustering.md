# Clustering

uptop supports three deployment modes for different reliability and coverage needs.

## Single node (default)

Out of the box, uptop runs as a standalone leader. One process, one database, runs all checks. No clustering config needed.

## Leader + follower (HA failover)

A follower is a standby replica that takes over if the leader goes down.

**How it works:**
- The follower polls the leader's `/api/health` endpoint every 5 seconds
- After 3 consecutive failures (15 seconds), the follower promotes itself and starts running checks
- When the leader recovers, the follower detects it and goes back to standby
- Both nodes have their own database — they do not share state

**Required env vars:**

| Node | Variable | Value |
|------|----------|-------|
| Both | `UPTOP_CLUSTER_SECRET` | Same shared secret |
| Follower | `UPTOP_CLUSTER_MODE` | `follower` |
| Follower | `UPTOP_PEER_URL` | Leader's HTTP URL (e.g. `http://leader:8080`) |

See [`deploy/docker-compose.cluster.yml`](../deploy/docker-compose.cluster.yml) for a working example.

## Leader + probes (distributed monitoring)

Probes are lightweight, stateless nodes that run checks from different locations and report results back to the leader.

**How it works:**
- A probe registers with the leader on startup
- Every 30 seconds, it fetches check assignments filtered by its region
- It runs the assigned checks (up to 10 concurrent) and posts results back
- The leader aggregates results from all probes and triggers alerts based on the aggregation strategy
- Probes have no database, no UI, and no configuration of their own

**Required env vars:**

| Node | Variable | Value |
|------|----------|-------|
| Both | `UPTOP_CLUSTER_SECRET` | Same shared secret |
| Leader | `UPTOP_AGG_STRATEGY` | `any-down`, `majority-down`, or `all-down` |
| Probe | `UPTOP_CLUSTER_MODE` | `probe` |
| Probe | `UPTOP_PEER_URL` | Leader's HTTP URL |
| Probe | `UPTOP_NODE_ID` | Unique identifier (e.g. `probe-us-east`) |
| Probe | `UPTOP_NODE_REGION` | Region tag matching monitor assignments |

Optional: `UPTOP_NODE_NAME` for a human-readable label in the TUI.

See [`deploy/docker-compose.probe.yml`](../deploy/docker-compose.probe.yml) for a multi-region example.

## Aggregation strategies

When multiple probes check the same monitor, the leader combines their results:

| Strategy | Behavior |
|----------|----------|
| `any-down` (default) | DOWN if **any** probe reports down |
| `majority-down` | DOWN if **most** probes report down |
| `all-down` | DOWN only if **all** probes report down |

Set via `UPTOP_AGG_STRATEGY` on the leader.

## Follower vs probe

| | Follower | Probe |
|---|---|---|
| **Purpose** | Failover / redundancy | Distributed checks from multiple regions |
| **Database** | Own database (independent) | None (stateless) |
| **Runs checks** | Only when leader is down | Always, on assigned monitors |
| **Scales to** | 1 follower per leader | Many probes per leader |

## Security

- Set `UPTOP_CLUSTER_SECRET` on all nodes. Without it, cluster API endpoints are unauthenticated.
- Secrets are sent in HTTP headers (`X-Upkeep-Secret`). Use TLS or a reverse proxy for production.
- uptop warns on startup if the cluster secret is missing or if cluster mode is active without TLS.
