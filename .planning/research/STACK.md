# Stack Research: VPS Management TUI Polish

**Research Date:** 2026-03-20
**Domain:** Terminal dashboard UI for infrastructure management
**Context:** Subsequent milestone — existing Go CLI/TUI with Cobra + Charm stack

## Current Stack (Already In Use)

| Component | Library | Version | Status |
|-----------|---------|---------|--------|
| CLI Framework | cobra | v1.10.2 | Keep |
| TUI Framework | bubbletea | v1.3.10 | Keep |
| TUI Forms | huh | v0.8.0 | Keep |
| TUI Styling | lipgloss | v1.1.0 | Keep |
| TUI Components | bubbles | v0.21.1 | Keep |
| Charts | ntcharts | v0.4.0 | Keep |
| Mouse Zones | bubblezone | latest | Keep |

**Verdict:** The Charm stack is the Go TUI ecosystem standard. No stack changes needed — this is about using what's already there more effectively.

## Recommendations for TUI Polish

### Layout & Dashboard

**lipgloss v1.1.0** — Already in use. Key features for dashboard:
- `lipgloss.JoinHorizontal()` / `lipgloss.JoinVertical()` for multi-pane layouts
- `lipgloss.Place()` for centering/positioning within panes
- Border styles for panel separation
- **Confidence:** High — already using it, just need to leverage layout features more

### Real-time Updates

**bubbletea tick commands** — Built-in to Bubbletea:
- `tea.Tick()` for periodic refresh (poll server status every 5-30s)
- `tea.Cmd` pattern for async API calls that don't block the UI
- Background data fetching with message-based updates
- **Confidence:** High — standard Bubbletea pattern

### Table Display

**bubbles/table** — Already available in bubbles v0.21.1:
- Sortable columns
- Row selection with keyboard navigation
- Custom cell rendering for status indicators
- **Confidence:** High — well-documented component

### Status Indicators

**lipgloss adaptive colors** — Use terminal-aware colors:
- Green/red/yellow for server status (running/stopped/transitioning)
- Unicode symbols: `●` running, `○` stopped, `◐` transitioning
- `lipgloss.AdaptiveColor{}` for light/dark terminal support
- **Confidence:** High

### Keyboard Shortcuts

**bubbles/key** — Key binding management:
- `key.NewBinding()` for action shortcuts
- Help view showing available shortcuts per context
- vim-style navigation (j/k, g/G)
- **Confidence:** High — standard pattern

## What NOT to Add

| Library | Why Not |
|---------|---------|
| tcell/tview | Competing TUI framework — Charm stack is better and already in use |
| termui | Outdated, less maintained than Charm stack |
| Additional HTTP clients | hcloud-go SDK handles Hetzner; use net/http for others |
| WebSocket libraries | Server status doesn't need WebSockets — polling is fine for VPS management |
| State management libs | Bubbletea's message passing IS the state management |

## Build Order Implications

1. **Layout system first** — Multi-pane dashboard layout using lipgloss
2. **Data layer** — Background refresh with tea.Cmd + tick
3. **Components** — Status indicators, tables, action panels
4. **Polish** — Keyboard shortcuts, help overlay, transitions

---
*Research completed: 2026-03-20*
