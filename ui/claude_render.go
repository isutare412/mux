package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/lunemis/mux/tmux"
)

// formatElapsed renders a duration like "0m12s", "3m08s", or "1h54m31s".
func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	return fmt.Sprintf("%dm%02ds", m, s)
}

// claudeStateGlyph returns the icon and color for a Claude state. ClaudeNone
// returns an empty icon.
func claudeStateGlyph(state tmux.ClaudeState) (string, lipgloss.Color) {
	switch state {
	case tmux.ClaudeWaiting:
		return "◆", colorClaudeWaiting
	case tmux.ClaudeWorking:
		return "▸", colorClaudeWorking
	case tmux.ClaudeIdle:
		return "✓", colorClaudeIdle
	default:
		return "", colorMuted
	}
}

// claudeStatePriority ranks states for window roll-up: Waiting > Working > Idle.
func claudeStatePriority(state tmux.ClaudeState) int {
	switch state {
	case tmux.ClaudeWaiting:
		return 3
	case tmux.ClaudeWorking:
		return 2
	case tmux.ClaudeIdle:
		return 1
	default:
		return 0
	}
}

// windowClaudeRollup picks the highest-priority Claude pane info among a
// window's panes. Returns ok=false when no pane has Claude info.
func windowClaudeRollup(panes []tmux.Pane, cache map[int]tmux.ClaudeInfo) (tmux.ClaudeInfo, bool) {
	var best tmux.ClaudeInfo
	found := false
	for _, p := range panes {
		info, ok := cache[p.PID]
		if !ok || info.State == tmux.ClaudeNone {
			continue
		}
		if !found || claudeStatePriority(info.State) > claudeStatePriority(best.State) {
			best = info
			found = true
		}
	}
	return best, found
}
