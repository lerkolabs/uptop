# Changelog

## [2026.06.1] — 2026-06-01

### Changed
- Container runs as non-root user `uptop` (UID/GID 1000) instead of root (#44)
- SSH host key relocated to `/data/.ssh/id_ed25519` for non-root compatibility (#44)
- Release workflow prunes dangling images and build cache after Docker push (#44)

### Added
- SBOM and provenance attestations on Docker images for supply chain compliance (#44)
- Entrypoint script with volume writability check and migration guidance (#44)

### Breaking
- Existing Docker volumes with root-owned files require migration before upgrading:
  `docker run --rm -v <volume>:/data alpine chown -R 1000:1000 /data`

## [2026.05.5] — 2026-05-29

### Added
- Error reason display when monitors go DOWN (#33)
- Push monitor lifecycle — PENDING, LATE, DOWN states (#34)
- Logs tab overhaul — severity tags, filtering, recovery durations (#35)
- Alert channel health indicator and test alerts (#36)
- TUI screenshots in `assets/` (#32)
- CI status badge in README

### Changed
- Visual polish — detail sections, column headers, alert detail (#37)
- README rewritten with hero image, badges, collapsible install sections (#32)
- Changelog rewritten to match actual CalVer tag history
- Migrated to `lerkolabs` org namespace (#38)
- Docker-compose files moved to `deploy/`

## [2026.05.4] — 2026-05-27

### Added
- SSH user seeding from `UPTOP_ADMIN_KEY` env var and `UPTOP_KEYS` file (#31)
- GoReleaser for binary releases
- govulncheck in CI pipeline
- Multi-arch Docker builds (amd64 + arm64)

### Changed
- CI overhaul — Go 1.26, build caching, streamlined pipeline (#30)
- Bumped golang.org/x/crypto v0.47.0 → v0.52.0
- Bumped Alpine 3.21 → 3.23

### Security
- Phase 1: SSRF protection, input validation, safe dial (#26)
- Phase 2: TLS hardening, auth bypass fixes, rate limiting (#27)
- Phase 3: Graceful degradation, connection limits, timeout enforcement (#28)
- Phase 4: Code quality, error handling, linter fixes (#29)

## [2026.05.3] — 2026-05-25

### Added
- Theme system with 5 dark palettes — Default, Dracula, Nord, Tokyo Night, Gruvbox (#24)
- `--version` flag with build metadata injection
- Gitea Actions CI pipeline — test + lint (#20)
- golangci-lint configuration
- Comprehensive test suite — 94 tests across monitor, server, cluster (#19)
- CONTRIBUTING.md and SECURITY.md

### Changed
- Renamed project from go-upkeep to uptop (#25)
- Updated LICENSE with dual copyright for independent fork

### Fixed
- Form validators scoped to relevant monitor types (#23)
- Graceful shutdown for HTTP, SSH servers and database (#19)
- Constant-time secret comparison, request size limits (#19)
- Check interval jitter to prevent thundering herd (#19)
- TUI visual polish — zebra striping, group icons, sparkline stats (#18)

## [2026.05.2] — 2026-05-22

### Added
- Incident management and maintenance windows (#17)
- Production docker-compose.yml

### Fixed
- Viewport sizing and dynamic chrome calculation (#16)
- Form height constrained to terminal with resize forwarding
- Maintenance'd monitors excluded from down count and pulse
- Group status correctly skips children in maintenance

## [2026.05.1] — 2026-05-16

### Added
- Distributed probing with leader + probe nodes
- Config-as-code — YAML apply/export with dry-run and prune
- TUI polish — status bar, tab badges, detail panel, modals
- DOWN-first sort, health pulse, site filter
- Type icons in sites table
- Sparkline history graphs
- Persistent state — uptime, status, latency, and logs survive restarts
- Push token stripping from /status/json response

## [2026.04.1] — 2026-04-01

### Added
- SSH-accessible TUI built on Bubble Tea + Wish
- 6 check types — HTTP, Push, Ping, Port, DNS, Group
- 9 alert providers — Discord, Slack, Email, Ntfy, Telegram, PagerDuty, Pushover, Gotify, Webhook
- SQLite and PostgreSQL support
- HA clustering with automatic failover
- Prometheus /metrics endpoint
- Public status page (HTML + JSON)
- Uptime Kuma backup import
