# Pitfalls Research: TUI Dashboard

**Research Date:** 2026-03-20
**Domain:** Terminal dashboard UI for infrastructure management
**Context:** Polishing existing Go Bubbletea TUI into dashboard experience

## Critical Pitfalls

### 1. Blocking the UI Thread with API Calls

**What goes wrong:** API calls to Hetzner/Cloudflare/Porkbun block the Bubbletea event loop, freezing the entire TUI. Users see a hung terminal.

**Warning signs:**
- TUI stops responding to keyboard input during data loads
- No loading indicators visible
- Users hitting Ctrl+C because they think it's stuck

**Prevention:**
- ALL API calls must go through `tea.Cmd` (never call provider methods in `Update()` directly)
- Show spinners/loading states for every async operation
- Set reasonable timeouts on API calls (10s default)

**Phase:** Should be addressed in Phase 1 (dashboard foundation)

### 2. Flicker on Refresh

**What goes wrong:** Auto-refresh redraws the entire view, causing visible flicker. Especially bad in split-pane layouts. Users find it distracting and it makes the tool feel cheap.

**Warning signs:**
- Screen blinks every refresh cycle
- Selected row jumps or loses highlight
- Scroll position resets on refresh

**Prevention:**
- Diff incoming data against current state — only update changed rows
- Preserve selection state (selected row ID, not index) across refreshes
- Use `lipgloss` rendering to only update changed panes
- Refresh interval 10-30s (not too frequent)

**Phase:** Should be addressed when implementing auto-refresh

### 3. Focus Management in Multi-Pane Layout

**What goes wrong:** Keyboard events go to the wrong pane. User thinks they're navigating the server list but they're actually in the detail pane. Tab/Shift+Tab behavior is inconsistent.

**Warning signs:**
- Key presses have unexpected effects
- User can't tell which pane is active
- No visual indicator of focused pane

**Prevention:**
- Clear visual focus indicator (highlighted border on active pane)
- Tab/Shift+Tab to switch panes consistently
- Each pane only processes keys when focused
- Test with different terminal emulators

**Phase:** Should be addressed in layout/navigation phase

### 4. Terminal Size Handling

**What goes wrong:** TUI breaks or looks terrible in small terminals. Panes overlap, text truncates mid-word, or the app crashes on resize.

**Warning signs:**
- Layout breaks below 80 columns
- Resize causes panic or garbled output
- Content overflows pane boundaries

**Prevention:**
- Handle `tea.WindowSizeMsg` in root model, propagate to all children
- Define responsive breakpoints (wide/medium/narrow)
- Test at 80x24 minimum (standard terminal size)
- Truncate long text with ellipsis, don't wrap

**Phase:** Should be addressed alongside layout system

### 5. State Inconsistency After Actions

**What goes wrong:** User reboots a server, status still shows "running" because the cache hasn't updated. Or action completes but the list doesn't reflect the change.

**Warning signs:**
- Server shows wrong status after action
- User has to manually refresh to see changes
- Action "succeeds" but nothing visibly changes

**Prevention:**
- After mutation: immediately set optimistic status ("rebooting...")
- Invalidate cache for affected resource
- Force refresh after action completion (don't wait for next tick)
- Show action progress inline on the affected row

**Phase:** Should be addressed with quick actions

### 6. Error Swallowing in Background Operations

**What goes wrong:** Background refresh fails silently. Auto-refresh error gets logged nowhere. User sees stale data without knowing it's stale.

**Warning signs:**
- Data stops updating but no error shown
- Stale timestamps not visible
- Retry storms consuming API rate limits

**Prevention:**
- Show subtle error indicator in status bar ("Last refresh failed, retrying...")
- Distinguish transient errors (retry) from auth errors (stop and prompt)
- Rate limit retries (don't spam API on repeated failures)
- Show "last updated: X ago" timestamp

**Phase:** Should be addressed with auto-refresh and error handling

### 7. Losing User Context on View Transitions

**What goes wrong:** User drills into server detail, goes back, and their scroll position / selection is gone. Or they switch tabs and lose their place.

**Warning signs:**
- Scroll position resets on back navigation
- Selected item deselects when returning to list
- Filter/search state lost on tab switch

**Prevention:**
- Each view model preserves its own state (selection, scroll, filters)
- Back navigation restores previous view state
- Tab switching preserves state per tab

**Phase:** Should be addressed in navigation/view router

### 8. Charm Stack Version Conflicts

**What goes wrong:** Updating one Charm library breaks another. Bubbletea, lipgloss, bubbles, and huh have interdependencies that aren't always obvious.

**Warning signs:**
- Build errors after updating a single Charm dependency
- Runtime panics in rendering
- Style rendering looks wrong after update

**Prevention:**
- Update all Charm packages together (they release in sync)
- Pin versions in go.mod
- Test TUI rendering after any dependency update
- Check Charm's changelog for breaking changes

**Phase:** Ongoing — address at project start

## Medium-Priority Pitfalls

### 9. SSH Quick Connect Edge Cases
- Terminal raw mode conflicts between Bubbletea and SSH
- Must properly suspend Bubbletea before launching SSH (`tea.ExecProcess`)
- Restore TUI state after SSH session ends

### 10. Color Support Across Terminals
- Not all terminals support 256 colors or true color
- Use `lipgloss.AdaptiveColor{}` for light/dark terminal support
- Test in: iTerm2, Terminal.app, Windows Terminal, basic Linux terminal
- Don't rely on emoji for critical status indicators (use ASCII fallbacks)

### 11. API Rate Limiting
- Auto-refresh with multiple users/instances can hit provider rate limits
- Hetzner: 3600 req/hour, Cloudflare: varies by plan
- Implement per-provider rate awareness
- Back off refresh interval when rate-limited

---
*Research completed: 2026-03-20*
