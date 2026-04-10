# Research Summary: VPSM TUI Polish

**Synthesized:** 2026-03-20
**Sources:** STACK.md, FEATURES.md, ARCHITECTURE.md, PITFALLS.md

## Key Findings

### Stack
The existing Charm stack (Bubbletea, Lipgloss, Huh, Bubbles) is the right choice — no stack changes needed. The work is about leveraging existing libraries more effectively: lipgloss for multi-pane layouts, bubbletea tick commands for auto-refresh, bubbles/table for sortable lists, and lipgloss adaptive colors for status indicators.

### Table Stakes Features
Users expect from a polished VPS TUI:
1. **Server overview dashboard** — all servers at a glance with status colors
2. **Keyboard navigation** — vim-style (j/k), Enter to drill in, Esc to go back
3. **Quick actions** — start/stop/reboot/SSH in 1-2 keystrokes
4. **Loading states** — spinners during API calls, never a frozen screen
5. **Responsive layout** — works in any terminal size
6. **Error display** — clear inline errors, no TUI crashes
7. **DNS record management** — view and manage records interactively
8. **CLI output quality** — consistent tables, JSON option, colors

### Differentiators
What would make VPSM stand out:
1. **Auto-refresh / live updates** — server status polls automatically
2. **Multi-pane dashboard** — lazydocker-style split view
3. **Server metrics** — CPU/RAM charts in terminal (ntcharts already in deps)
4. **Inline DNS editing** — edit records without leaving TUI
5. **Context-aware help** — ? overlay showing actions for current view

### Architecture Approach
- **Unified app model** in `internal/tui/` composing domain-specific views
- **Tab-based navigation**: Servers | DNS | SSH Keys
- **Split-pane layout**: list + detail side by side (wide), stacked (narrow)
- **Message-based data flow**: all API calls via `tea.Cmd`, never blocking UI
- **Gradual migration**: existing TUI views become "views" within the dashboard

### Critical Pitfalls to Avoid
1. **Blocking UI with API calls** — always use `tea.Cmd` for async
2. **Flicker on refresh** — diff data, preserve selection state
3. **Focus confusion in multi-pane** — clear visual indicator, consistent Tab behavior
4. **Terminal size crashes** — handle `WindowSizeMsg`, test at 80x24 minimum
5. **Stale data after actions** — optimistic updates + forced refresh after mutations

### Recommended Build Order
1. Shared TUI infrastructure (layout, navigation, status bar, key bindings)
2. Server dashboard (list + detail panes, status indicators)
3. Quick actions (start/stop/reboot/SSH from dashboard)
4. Auto-refresh (background polling, diff-based updates)
5. DNS management (domain list, record table, inline editing)
6. Polish (responsive layout, help overlay, transitions, metrics)

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| UI blocking during API calls | High if not careful | High — frozen TUI | Enforce tea.Cmd pattern from day 1 |
| Flicker on auto-refresh | Medium | Medium — feels cheap | Diff-based updates, preserve selection |
| Focus management bugs | Medium | Medium — confusing UX | Clear focus indicators, consistent Tab |
| Terminal compatibility | Low | Medium — broken for some users | Test on multiple terminals |
| Charm version conflicts | Low | Low — fixable | Update all Charm deps together |

## Confidence Assessment

| Area | Confidence | Notes |
|------|-----------|-------|
| Stack choice | High | Charm stack is established, no changes needed |
| Architecture pattern | High | lazydocker/k9s pattern is proven |
| Feature prioritization | High | Clear table stakes vs differentiators |
| Build order | Medium | May need adjustment based on existing TUI code complexity |

---
*Summary synthesized: 2026-03-20*
