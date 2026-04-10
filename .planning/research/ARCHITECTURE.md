# Architecture Research: TUI Dashboard

**Research Date:** 2026-03-20
**Domain:** Terminal dashboard architecture for infrastructure management
**Context:** Existing domain-first provider architecture, adding polished TUI dashboard layer

## Dashboard Architecture Pattern

### Component Model

```
┌─────────────────────────────────────────────────────┐
│                    App Model                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │
│  │ Nav Bar  │  │ View     │  │ Status Bar       │  │
│  │          │  │ Router   │  │ (errors, hints)  │  │
│  └──────────┘  └──────────┘  └──────────────────┘  │
│                     │                                │
│  ┌──────────────────┼──────────────────────┐        │
│  │                  │                      │        │
│  ▼                  ▼                      ▼        │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │
│  │ Server   │  │ DNS      │  │ SSH Key          │  │
│  │ Dashboard│  │ Manager  │  │ Manager          │  │
│  └──────────┘  └──────────┘  └──────────────────┘  │
│       │              │              │                │
│       ▼              ▼              ▼                │
│  ┌──────────────────────────────────────────────┐   │
│  │            Data Layer (Services)              │   │
│  │  - Background refresh via tea.Cmd             │   │
│  │  - Cached data with SWR                       │   │
│  │  - Action tracking                            │   │
│  └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

### Key Components

**1. App Model (Root)**
- Owns all child models
- Routes key events to focused view
- Manages global state (current provider, terminal size)
- Handles view transitions via messages

**2. Navigation**
- Tab-based top navigation: Servers | DNS | SSH Keys
- Breadcrumb for drill-down: Servers > server-01 > Metrics
- Keyboard shortcuts: 1/2/3 for tabs, Esc to go back

**3. View Router**
- Maps navigation state to active view model
- Handles focus management between panes
- Coordinates transitions with loading states

**4. Dashboard Views (per domain)**
- Server Dashboard: list pane + detail pane (split horizontal)
- DNS Manager: domain list + record table
- SSH Key Manager: key list with actions

**5. Data Layer**
- Background polling via `tea.Tick` (configurable interval)
- Services wrap providers with caching
- Mutations invalidate cache and trigger refresh
- Action tracking for long-running operations

## Data Flow

### Initial Load
1. App starts → sends `FetchServersMsg` command
2. Shows loading spinner in server list pane
3. Service fetches from provider (or SWR cache)
4. Returns `ServersLoadedMsg` with server slice
5. Server list renders with status indicators

### Auto-Refresh
1. `tea.Tick(refreshInterval)` fires `RefreshTickMsg`
2. App sends background `FetchServersMsg`
3. On response, diff with current state
4. Update only changed rows (avoid flicker)
5. Schedule next tick

### User Action (e.g., reboot)
1. User presses 'r' on selected server
2. Confirmation prompt shown
3. On confirm: send `RebootServerMsg` command
4. Show action indicator on that server row
5. Action tracking polls for completion
6. On complete: refresh server status, show success

### View Transition
1. User presses Enter on server row
2. `NavigateMsg{view: ServerDetail, id: serverID}` sent
3. Router switches active view to detail pane
4. Detail view sends `FetchServerDetailMsg`
5. Breadcrumb updates

## Bubbletea Message Types

```
// Navigation
NavigateMsg{view, params}
BackMsg{}
TabSwitchMsg{tab}

// Data
FetchServersMsg{}
ServersLoadedMsg{servers, err}
FetchDNSRecordsMsg{domain}
DNSRecordsLoadedMsg{records, err}
RefreshTickMsg{}

// Actions
ServerActionMsg{id, action}
ActionProgressMsg{id, status}
ActionCompleteMsg{id, result}

// UI
WindowSizeMsg{w, h}
ErrorMsg{err, dismissable}
SuccessMsg{text, duration}
```

## Layout Strategy

### Split Pane (lazydocker-style)
```
┌─────────────────────┬──────────────────────────────┐
│ Servers             │ Server Detail                │
│ ─────────────       │ ────────────────             │
│ ● server-01  [HEL1] │ Name: server-01             │
│ ○ server-02  [FSN1] │ Status: Running              │
│ ● server-03  [NBG1] │ IP: 116.203.x.x             │
│                     │ Type: CX22                   │
│                     │ Location: Helsinki           │
│                     │                              │
│                     │ [s]SH [r]eboot [p]ower      │
│                     │ [d]elete [m]etrics           │
├─────────────────────┴──────────────────────────────┤
│ ● 3 servers | Hetzner | ? help | q quit           │
└────────────────────────────────────────────────────┘
```

### Responsive Breakpoints
- **Wide (>120 cols):** Side-by-side list + detail
- **Medium (80-120 cols):** List only, Enter for detail overlay
- **Narrow (<80 cols):** Compact list, minimal columns

## Build Order (Dependencies)

1. **Shared TUI infrastructure** — Layout system, navigation, status bar, key bindings
2. **Server dashboard** — List pane with status indicators, basic detail view
3. **Quick actions** — Start/stop/reboot/SSH from dashboard
4. **Auto-refresh** — Background polling, diff-based updates
5. **DNS management** — Domain list, record table, inline editing
6. **Polish** — Responsive layout, help overlay, transitions, error handling

## Integration with Existing Architecture

The existing code has TUI per domain (`internal/server/tui/`, `internal/dns/tui/`). The dashboard approach needs:

- **New unified app model** in `internal/tui/` that composes domain TUI models
- **Keep domain TUI models** but adapt them as "views" within the dashboard
- **Shared components** in `internal/tui/` (status bar, navigation, layout helpers)
- **Gradual migration** — existing TUI views can work standalone AND within dashboard

---
*Research completed: 2026-03-20*
