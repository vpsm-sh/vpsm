# Features Research: VPS Management CLI/TUI

**Research Date:** 2026-03-20
**Domain:** VPS infrastructure management tools
**Context:** Subsequent milestone — polish TUI dashboard for existing CLI tool

## Reference Products

- **lazydocker** — Docker TUI dashboard (gold standard for infrastructure TUI UX)
- **k9s** — Kubernetes TUI (dashboard-style, real-time updates)
- **hcloud CLI** — Hetzner's official CLI (good CLI UX reference)
- **doctl** — DigitalOcean CLI (comprehensive CLI reference)
- **htop/btop** — System monitoring TUI (layout and refresh patterns)

## Table Stakes (Must Have)

Features users expect from a polished infrastructure TUI. Missing these = tool feels unfinished.

### Server Overview Dashboard
- **Description:** Single view showing all servers with status, IP, type, location
- **Complexity:** Medium
- **Dependencies:** Provider list API, table component
- **User expectation:** "I open the tool and see my infrastructure at a glance"

### Status Indicators
- **Description:** Visual server status (running/stopped/rebuilding) with color coding
- **Complexity:** Low
- **Dependencies:** Server status from provider API
- **User expectation:** "I can instantly tell which servers are up or down"

### Keyboard Navigation
- **Description:** vim-style navigation (j/k), Enter to drill in, Esc to go back, ? for help
- **Complexity:** Medium
- **Dependencies:** Key binding framework (bubbles/key)
- **User expectation:** "I can navigate without touching the mouse"

### Quick Actions
- **Description:** Start/stop/reboot from server list without navigating away
- **Complexity:** Medium
- **Dependencies:** Server status dashboard, action tracking
- **User expectation:** "I can take action on a server in 1-2 keystrokes"

### SSH Quick Connect
- **Description:** Press a key to SSH into selected server
- **Complexity:** Low
- **Dependencies:** Server IP + stored SSH username
- **User expectation:** "I select a server and press 's' to SSH in"

### Responsive Layout
- **Description:** TUI adapts to terminal size, handles resize gracefully
- **Complexity:** Medium
- **Dependencies:** lipgloss layout, WindowSizeMsg handling
- **User expectation:** "Tool works in any terminal size without breaking"

### Loading States
- **Description:** Spinners/indicators during API calls, not frozen screen
- **Complexity:** Low
- **Dependencies:** Bubbletea async commands
- **User expectation:** "I know the tool is working, not stuck"

### Error Display
- **Description:** Clear inline error messages that don't crash the TUI
- **Complexity:** Low
- **Dependencies:** Error handling in TUI layer
- **User expectation:** "If something fails, I see why and can retry"

### DNS Record List
- **Description:** View DNS records for a domain in table format
- **Complexity:** Medium
- **Dependencies:** DNS provider, domain selection
- **User expectation:** "I can see all my DNS records organized by domain"

### CLI Output Quality
- **Description:** Consistent table formatting, JSON output option, color support
- **Complexity:** Low
- **Dependencies:** Output formatting utilities
- **User expectation:** "CLI output is clean and scriptable"

## Differentiators (Competitive Advantage)

### Auto-Refresh / Live Updates
- **Description:** Server status auto-refreshes every N seconds without user action
- **Complexity:** Medium
- **Dependencies:** Tick-based polling, background API calls
- **Why differentiating:** Most CLIs are one-shot; live updates feel modern

### Multi-Pane Dashboard
- **Description:** Side-by-side panels (server list + detail view, or servers + DNS)
- **Complexity:** High
- **Dependencies:** lipgloss layout system, focus management
- **Why differentiating:** Goes beyond basic list/detail — feels like a real dashboard

### Server Metrics View
- **Description:** CPU/RAM/disk/bandwidth charts for selected server
- **Complexity:** Medium
- **Dependencies:** MetricsProvider interface, ntcharts
- **Why differentiating:** Not available in most CLI tools, bridges gap to web dashboards

### Inline DNS Editing
- **Description:** Edit DNS records directly in TUI without switching to CLI commands
- **Complexity:** Medium
- **Dependencies:** DNS service layer, form components (huh)
- **Why differentiating:** Removes context switching between DNS and server management

### Context-Aware Help
- **Description:** Help overlay showing available actions for current view/selection
- **Complexity:** Low
- **Dependencies:** Key binding definitions per view
- **Why differentiating:** Reduces learning curve for new users

### Provider Switching
- **Description:** Switch between providers within TUI (e.g., view Hetzner then DigitalOcean)
- **Complexity:** Low (architecture supports it)
- **Dependencies:** Provider registry
- **Why differentiating:** Unified multi-cloud view

## Anti-Features (Don't Build)

| Feature | Why Not |
|---------|---------|
| Real-time WebSocket streaming | VPS status doesn't change fast enough — polling every 10-30s is fine |
| In-TUI file editor | Scope creep — SSH into the server instead |
| Log viewer | Too complex, better tools exist (stern, lnav) |
| Cost calculator | Provider-specific, hard to maintain accuracy |
| Team collaboration | v1 is single-user, adds massive complexity |
| Notification system | Desktop notifications for server events — overengineered for CLI tool |
| Server provisioning wizard | Complex multi-step flow — keep create simple, use CLI for advanced options |

## Feature Dependencies

```
Loading States ──► Server Dashboard ──► Quick Actions
                                    ──► Auto-Refresh
                                    ──► SSH Quick Connect

Keyboard Navigation ──► All TUI Views

Responsive Layout ──► Multi-Pane Dashboard

DNS Record List ──► Inline DNS Editing

Server Dashboard ──► Server Metrics View
```

---
*Research completed: 2026-03-20*
