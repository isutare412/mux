package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/lunemis/mux/tmux"
)

var (
	// Colors
	colorPrimary  = lipgloss.Color("#7C3AED")
	colorAccent   = lipgloss.Color("#22D3EE")
	colorSuccess  = lipgloss.Color("#22C55E")
	colorDanger   = lipgloss.Color("#EF4444")
	colorMuted    = lipgloss.Color("#6B7280")
	colorBorder   = lipgloss.Color("#374151")
	colorSelected = lipgloss.Color("#312E81")
	colorCursor   = lipgloss.Color("#A78BFA")

	// colorSessionName is the bright yellow used for session names so they stand
	// out from window/pane rows. Kept distinct from the claude orange (#F59E0B).
	colorSessionName = lipgloss.Color("#FACC15") // yellow-400

	// Claude state colors
	colorClaudeWorking = lipgloss.Color("#60A5FA") // blue
	colorClaudeWaiting = lipgloss.Color("#F59E0B") // orange
	colorClaudeIdle    = lipgloss.Color("#22C55E") // green

	// colorClaude is claude's brand orange for window names of windows running
	// claude. Sourced from the canonical AI-tool registry so it cannot drift
	// from the claude icon color.
	colorClaude = lipgloss.Color(claudeBrandColorHex())

	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorDanger).
			Bold(true)

	inputLabelStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)
)

// claudeBrandColorHex returns claude's brand color hex from the AI-tool
// registry, the single source of truth shared with the claude icon. Returns
// "" if claude is somehow unregistered, which yields an unstyled name.
func claudeBrandColorHex() string {
	tool, _ := tmux.LookupAITool("claude")
	return tool.Color
}
