# Requirements: VPSM

**Defined:** 2026-03-20
**Core Value:** Users can manage their VPS servers and DNS records across providers from a single, polished command-line tool

## v1 Requirements

Requirements for TUI polish milestone. Each maps to roadmap phases.

### TUI Infrastructure

- [ ] **TUII-01**: User can switch between Servers, DNS, and SSH Keys views via tab navigation
- [ ] **TUII-02**: Dashboard uses multi-pane layout (list + detail side-by-side when terminal is wide enough)
- [ ] **TUII-03**: Status bar shows current provider, error messages, and action hints
- [ ] **TUII-04**: User can press ? to see available keyboard shortcuts for current view

### Server Dashboard

- [ ] **SRVD-01**: User can see all servers in a list with color-coded status indicators (running/stopped/transitioning)
- [ ] **SRVD-02**: User can select a server to see its details (name, status, IP, type, location, created date)
- [ ] **SRVD-03**: User can start, stop, or reboot a server from the dashboard with hotkeys
- [ ] **SRVD-04**: User can SSH into a selected server from the dashboard with a single keypress
- [ ] **SRVD-05**: Server statuses auto-refresh every 10-30 seconds without user action

### DNS Management

- [ ] **DNSM-01**: User can view a list of domains from their DNS provider
- [ ] **DNSM-02**: User can view DNS records for a selected domain in table format
- [ ] **DNSM-03**: User can create new DNS records interactively in the TUI
- [ ] **DNSM-04**: User can edit existing DNS records interactively in the TUI
- [ ] **DNSM-05**: User can delete DNS records interactively in the TUI
- [ ] **DNSM-06**: User can search for available domains via their DNS provider

### UX Polish

- [ ] **UXPL-01**: User can navigate all views with vim-style keys (j/k, Enter to select, Esc to go back)
- [ ] **UXPL-02**: API errors display inline without crashing the TUI, with retry hints
- [ ] **UXPL-03**: All API operations show loading spinners/indicators
- [ ] **UXPL-04**: TUI layout adapts to terminal size with responsive breakpoints

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Server Features

- **SRVF-01**: User can view server metrics charts (CPU/RAM/bandwidth) in TUI
- **SRVF-02**: User can create new servers from TUI with guided wizard
- **SRVF-03**: User can manage server firewall rules from TUI

### Additional Services

- **ASVC-01**: User can manage volumes (create, attach, detach, delete)
- **ASVC-02**: User can manage S3/object storage buckets
- **ASVC-03**: User can manage additional VPS providers (DigitalOcean, Vultr, Linode)

### Advanced UX

- **AUXP-01**: Inline DNS record editing directly in table
- **AUXP-02**: Smooth view transitions and animations
- **AUXP-03**: Provider switching within TUI

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Web UI | CLI-first tool, TUI covers interactive use cases |
| Real-time WebSocket streaming | VPS status doesn't change fast enough — polling is sufficient |
| In-TUI file editor | Scope creep — SSH into server instead |
| Log viewer | Better dedicated tools exist (stern, lnav) |
| Cost calculator | Provider-specific, hard to maintain accuracy |
| Team collaboration | v1 is single-user, adds massive complexity |
| Desktop notifications | Overengineered for CLI tool |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| TUII-01 | — | Pending |
| TUII-02 | — | Pending |
| TUII-03 | — | Pending |
| TUII-04 | — | Pending |
| SRVD-01 | — | Pending |
| SRVD-02 | — | Pending |
| SRVD-03 | — | Pending |
| SRVD-04 | — | Pending |
| SRVD-05 | — | Pending |
| DNSM-01 | — | Pending |
| DNSM-02 | — | Pending |
| DNSM-03 | — | Pending |
| DNSM-04 | — | Pending |
| DNSM-05 | — | Pending |
| DNSM-06 | — | Pending |
| UXPL-01 | — | Pending |
| UXPL-02 | — | Pending |
| UXPL-03 | — | Pending |
| UXPL-04 | — | Pending |

**Coverage:**
- v1 requirements: 19 total
- Mapped to phases: 0
- Unmapped: 19 ⚠️

---
*Requirements defined: 2026-03-20*
*Last updated: 2026-03-20 after initial definition*
