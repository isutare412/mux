package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lunemis/mux/tmux"
)

const (
	indentWindow = 2
	indentPane   = 4
)

// renderListView renders the flattened tree (sessions + expanded windows + panes).
// Items must already be flattened by the caller via flatten().
func renderListView(items []listItem, cursor int, filter string, t *treeState, width, height int, labels []string, jumpActive bool) string {
	innerWidth := width - 2 // border chars
	innerHeight := height - 2

	if len(items) == 0 {
		msg := "No tmux sessions found"
		if filter != "" {
			msg = fmt.Sprintf("No match: \"%s\"", filter)
		}
		lines := make([]string, innerHeight)
		mid := innerHeight / 2
		for i := range lines {
			if i == mid {
				lines[i] = padOrTruncate(centerText(msg, innerWidth), innerWidth)
			} else {
				lines[i] = strings.Repeat(" ", innerWidth)
			}
		}
		content := strings.Join(lines, "\n")
		return drawBorder(content, width, innerHeight)
	}

	offset := 0
	if cursor >= innerHeight {
		offset = cursor - innerHeight + 1
	}

	lines := make([]string, innerHeight)
	for i := 0; i < innerHeight; i++ {
		idx := i + offset
		if idx < len(items) {
			label := ""
			if jumpActive && idx < len(labels) {
				label = labels[idx]
			}
			lines[i] = formatItemRow(items[idx], idx == cursor, innerWidth, t, label)
		} else {
			lines[i] = strings.Repeat(" ", innerWidth)
		}
	}

	content := strings.Join(lines, "\n")
	return drawBorder(content, width, innerHeight)
}

// renderSessionList preserves the legacy session-only renderer for tests and
// callers that don't need tree expansion. It wraps each session in a listItem
// and delegates to renderListView with an empty tree state.
func renderSessionList(sessions []tmux.Session, cursor int, filter string, width, height int) string {
	items := make([]listItem, len(sessions))
	for i := range sessions {
		items[i] = listItem{kind: itemSession, session: &sessions[i]}
	}
	state := newTreeState()
	return renderListView(items, cursor, filter, &state, width, height, nil, false)
}

// styleRow applies base to text. When label is non-empty, the first cell of text
// is replaced by the jump label, rendered with the accent color on top of base
// (so it keeps base's background — e.g. the cursor row's highlight). The label and
// the remainder are each rendered self-contained, so the label's SGR reset never
// leaks into the rest of the row.
func styleRow(text string, base lipgloss.Style, label string) string {
	if label == "" {
		return base.Render(text)
	}
	_, size := utf8.DecodeRuneInString(text)
	labelStyle := base.Bold(true).Foreground(colorAccent)
	return labelStyle.Render(label) + base.Render(text[size:])
}

func formatItemRow(it listItem, selected bool, width int, t *treeState, label string) string {
	switch it.kind {
	case itemWindow:
		expanded := t.isWindowExpanded(it.session.Name, it.window.Index)
		return formatWindowRow(it.session.Name, it.window, expanded, selected, width, t, label)
	case itemPane:
		return formatPaneRow(it.pane, selected, width, t)
	default:
		expanded := t.isSessionExpanded(it.session.Name)
		return formatSessionRow(*it.session, expanded, selected, width, label)
	}
}

func formatSessionRow(s tmux.Session, expanded, selected bool, width int, label string) string {
	chevron := "▶"
	if expanded {
		chevron = "▼"
	}

	status := "○"
	if s.Attached {
		status = "*"
	}

	name := s.Name
	if len(name) > maxSessionNameDisplay {
		name = name[:maxSessionNameDisplay-3] + "..."
	}

	ago := timeAgo(s.Created)

	icon, iconColor := commandIconPlain(s.ActiveCommand)
	var styledIcon string
	if iconColor != "" {
		styledIcon = " " + lipgloss.NewStyle().Foreground(lipgloss.Color(iconColor)).Render(icon)
	}

	branch := ""
	if s.GitBranch != "" {
		branch = " " + s.GitBranch
	}

	// Bold the session name on non-selected rows. The selected row gets its bold
	// treatment from the whole-row style below, so leave the name plain there to
	// avoid double-wrapping. Pad the name manually (instead of %-18s) because fmt
	// counts the embedded ANSI bytes as characters, which would break alignment.
	nameField := fmt.Sprintf("%-*s", maxSessionNameDisplay, name)
	if !selected {
		pad := strings.Repeat(" ", maxSessionNameDisplay-ansi.StringWidth(name))
		nameField = lipgloss.NewStyle().Bold(true).Render(name) + pad
	}

	text := fmt.Sprintf("%s %s %s %s", chevron, status, nameField, ago)
	text += styledIcon + branch
	extraWidth := 0
	if iconColor != "" {
		extraWidth = 1
	}
	row := padOrTruncate(text, width-extraWidth)

	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	if selected {
		base = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCursor).
			Background(colorSelected)
	}
	return styleRow(row, base, label)
}

func formatWindowRow(sessionName string, w *tmux.Window, expanded, selected bool, width int, t *treeState, label string) string {
	chevron := "▶"
	if expanded {
		chevron = "▼"
	}
	marker := " "
	if w.Active {
		marker = "*"
	}

	panes := t.panesCache[paneCacheKey{session: sessionName, window: w.Index}]
	info, isClaude := windowClaudeRollup(panes, t.claudeCache)

	// Color the window name claude-orange when the window runs claude, but only
	// on non-selected rows — the selected row's whole-row style takes over.
	name := w.Name
	if isClaude && !selected {
		name = lipgloss.NewStyle().Foreground(colorClaude).Render(w.Name)
	}

	text := fmt.Sprintf("%s%s %s %d:%s", strings.Repeat(" ", indentWindow), chevron, marker, w.Index, name)

	if isClaude {
		text += claudeSuffix(info)
	}

	row := padOrTruncate(text, width)

	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	if selected {
		base = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCursor).
			Background(colorSelected)
	}
	return styleRow(row, base, label)
}

func formatPaneRow(p *tmux.Pane, selected bool, width int, t *treeState) string {
	marker := " "
	if p.Active {
		marker = "*"
	}

	text := fmt.Sprintf("%s%s %d %s", strings.Repeat(" ", indentPane), marker, p.Index, p.Command)

	if info, ok := t.claudeInfo(p.PID); ok && info.State != tmux.ClaudeNone {
		text += claudeSuffix(info)
	}

	row := padOrTruncate(text, width)

	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	if selected {
		base = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCursor).
			Background(colorSelected)
	}
	return styleRow(row, base, "")
}

// claudeSuffix renders " <icon> <elapsed>  <recap>" for a Claude pane. The whole
// row is truncated to width by the caller, so recap is left intact here.
func claudeSuffix(info tmux.ClaudeInfo) string {
	icon, color := claudeStateGlyph(info.State)
	elapsed := ""
	if !info.Since.IsZero() {
		elapsed = formatElapsed(time.Since(info.Since))
	}
	styledIcon := lipgloss.NewStyle().Foreground(color).Render(icon)
	out := "  " + styledIcon
	if elapsed != "" {
		out += " " + elapsed
	}
	if info.Recap != "" {
		out += "  " + lipgloss.NewStyle().Foreground(color).Render(info.Recap)
	}
	return out
}

// commandIconPlain returns the raw icon and its color for known AI CLIs.
// Returns empty strings for non-AI commands.
func commandIconPlain(cmd string) (icon string, color string) {
	if tool, ok := tmux.LookupAITool(cmd); ok {
		return tool.Icon, tool.Color
	}
	return "", ""
}

func centerText(s string, width int) string {
	pad := (width - len(s)) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + s
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%3ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%3dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%3dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%3dd", int(d.Hours()/24))
	}
}
