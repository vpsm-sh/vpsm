# VPSM

## What This Is

A CLI-first tool for managing VPS infrastructure and related services (DNS, Volumes, S3) across multiple cloud providers. Built in Go with Cobra for CLI commands and the Charm stack (Bubbletea, Lipgloss, Huh) for interactive TUI experiences. Designed as an open-source tool for anyone managing VPS infrastructure.

## Core Value

Users can manage their VPS servers and DNS records across providers from a single, polished command-line tool — without switching between provider dashboards.

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

- ✓ CLI command structure with Cobra (server, dns, sshkey, auth, config commands) — existing
- ✓ Multi-provider architecture with factory registry pattern — existing
- ✓ Hetzner VPS provider (list, create, delete, start, stop, reboot, show) — existing
- ✓ Cloudflare DNS provider (list, create, update, delete records) — existing
- ✓ Porkbun DNS provider (list, create, update, delete records) — existing
- ✓ Hetzner SSH key management — existing
- ✓ OS keyring credential storage (macOS, Linux, Windows) — existing
- ✓ Auth commands (login, logout, status per provider) — existing
- ✓ Config system with default provider selection — existing
- ✓ DNS input validation and normalization via service layer — existing
- ✓ SWR caching for DNS reads — existing
- ✓ Action tracking with SQLite persistence for server operations — existing
- ✓ Basic server TUI (list, show, create, delete views) — existing
- ✓ Basic DNS TUI (interactive record management) — existing
- ✓ SSH key TUI (list, create, delete) — existing
- ✓ Retry with exponential backoff for transient errors — existing
- ✓ Cross-platform release builds (linux/darwin/windows) — existing

### Active

<!-- Current scope. Building toward these. -->

- [ ] Polished dashboard-style TUI with server status overview at a glance
- [ ] Quick actions from TUI (start/stop/reboot/SSH without leaving TUI)
- [ ] Interactive DNS record management in TUI
- [ ] Live status updates and auto-refresh in TUI
- [ ] Great overall user experience across CLI and TUI

### Out of Scope

<!-- Explicit boundaries. Includes reasoning to prevent re-adding. -->

- Volumes management — defer to post-v1, focus on polishing VPS + DNS first
- S3/object storage — defer to post-v1
- Firewall management — defer to post-v1
- Additional VPS providers (DigitalOcean, Vultr, Linode) — architecture supports it, but polish existing before expanding
- Web UI — CLI-first tool, TUI covers interactive use cases
- Multi-user / team features — v1 is single-user

## Context

- Early prototype stage — structure and provider architecture in place but TUI needs polish
- CLI is the primary interface; TUI is a nice-to-have layer on top
- Built with Charm stack (Bubbletea, Lipgloss, Huh, Bubbles) for TUI
- Domain-first architecture with vertical slices per service (server, dns, sshkey)
- Provider pattern allows adding new providers without touching business logic
- Existing codebase map at `.planning/codebase/` with detailed architecture docs
- Target audience: open source community managing VPS infrastructure

## Constraints

- **Tech stack**: Go with Cobra CLI + Charm TUI stack — established, not changing
- **Provider architecture**: Domain-first with factory registry — established pattern
- **Credential storage**: OS keyring only — no file-based token storage
- **Build**: CGO_ENABLED=0 for static cross-platform binaries

## Key Decisions

<!-- Decisions that constrain future work. Add throughout project lifecycle. -->

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| CLI-first, TUI secondary | CLI covers scripting/automation, TUI adds interactive convenience | — Pending |
| Dashboard-style TUI | Users want at-a-glance overview of all servers/services | — Pending |
| Polish VPS + DNS for v1 | Ship fewer services well rather than many services poorly | — Pending |
| Multi-provider from the start | Architecture already supports it, validates the abstraction | ✓ Good |

---
*Last updated: 2026-03-20 after initialization*
