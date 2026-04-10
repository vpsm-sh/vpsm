// Package components provides reusable Bubbletea UI building blocks for
// the vpsm TUI. These are render-only helpers (not tea.Model) used by
// the main TUI models to compose views.
package components

import (
	"strings"

	"nathanbeddoewebdev/vpsm/internal/tui/styles"

	"github.com/charmbracelet/lipgloss"
)

// Header renders the application header bar.
//
//	┌──────────────────────────────────────────┐
//	│  vpsm > server list            Hetzner   │
//	└──────────────────────────────────────────┘
func Header(width int, breadcrumb string, provider string) string {
	if width < 10 {
		return ""
	}

	leftStyle := styles.Title.Foreground(styles.Blue)
	left := leftStyle.Render("vpsm")
	if breadcrumb != "" {
		left += styles.MutedText.Render(" > ") + styles.Title.Render(breadcrumb)
	}

	right := ""
	if provider != "" {
		right = styles.Subtitle.Render(provider)
	}

	// Calculate spacing between left and right.
	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(right)
	innerWidth := width - 4 // account for padding
	gap := max(innerWidth-leftLen-rightLen, 1)

	content := left + strings.Repeat(" ", gap) + right

	bar := lipgloss.NewStyle().
		Width(width).
		Padding(0, 2).
		BorderStyle(lipgloss.Border{Bottom: "─"}).
		BorderBottom(true).
		BorderForeground(styles.DimGray).
		Render(content)

	return bar
}
