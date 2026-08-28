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

// listScrollOffset returns the index of the first visible row when the cursor
// sits at cursor and the viewport is rows lines tall. The list scrolls only far
// enough to keep the cursor on screen, so rows above stay put until they must
// move. The mouse hit test resolves clicks through the same function, which is
// why it lives outside the renderer.
func listScrollOffset(cursor, rows int) int {
	if cursor >= rows {
		return cursor - rows + 1
	}
	return 0
}

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

	offset := listScrollOffset(cursor, innerHeight)

	lines := make([]string, innerHeight)
	for i := 0; i < innerHeight; i++ {
		idx := i + offset
		if idx < len(items) {
			label := ""
			if idx < len(labels) {
				label = labels[idx]
			}
			lines[i] = formatItemRow(items[idx], idx == cursor, innerWidth, t, label, jumpActive)
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

// rowSegment is one run of a list row that shares a foreground color and weight.
// An empty fg inherits the row's base foreground; bold only ever adds weight on
// top of the base, never removes it.
//
// Rows are assembled as plain-text segments and styled once, by renderRow. The
// alternative — pre-rendering a colored piece and splicing it into the row text
// — emits a bare SGR reset in the middle of the line, which drops the selected
// row's background from that point on and leaves the highlight bar ending at
// whatever badge happens to come first.
type rowSegment struct {
	text string
	fg   lipgloss.Color
	bold bool
}

// rowBaseStyle returns the base style for a list row: the cursor highlight when
// selected, otherwise the given foreground. Segments layer their own color and
// weight on top of this.
func rowBaseStyle(selected bool, fg lipgloss.Color) lipgloss.Style {
	if selected {
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCursor).
			Background(colorSelected)
	}
	return lipgloss.NewStyle().Foreground(fg)
}

// segStyle layers a segment's own color and weight onto the row's base style.
func segStyle(base lipgloss.Style, s rowSegment) lipgloss.Style {
	style := base
	if s.fg != "" {
		style = style.Foreground(s.fg)
	}
	if s.bold {
		style = style.Bold(true)
	}
	return style
}

// applyJumpLabel swaps the row's first cell for the jump label, so the label
// costs no width. The label is muted (colorMuted) at rest and bold accent
// (colorAccent) in jump mode — except the inactive-anchor dot, which is not a
// key you can press and therefore stays muted in every mode.
func applyJumpLabel(segs []rowSegment, label string, active bool) []rowSegment {
	if label == "" {
		return segs
	}
	for i, s := range segs {
		if s.text == "" {
			continue
		}
		labelSeg := rowSegment{text: label, fg: colorMuted}
		if active && label != jumpInactiveLabel {
			labelSeg.fg = colorAccent
			labelSeg.bold = true
		}
		_, size := utf8.DecodeRuneInString(s.text)
		out := make([]rowSegment, 0, len(segs)+1)
		out = append(out, labelSeg, rowSegment{text: s.text[size:], fg: s.fg, bold: s.bold})
		return append(out, segs[i+1:]...)
	}
	return segs
}

// renderRow lays segments out into a row exactly width cells wide and paints
// them on top of base. Every segment — and the trailing padding — is rendered
// through base, so a selected row's background runs the full width instead of
// stopping at the first colored badge.
func renderRow(segs []rowSegment, base lipgloss.Style, width int, label string, active bool) string {
	if width <= 0 {
		return ""
	}
	segs = applyJumpLabel(segs, label, active)

	var b strings.Builder
	remaining := width
	var pending rowSegment
	flush := func() {
		if pending.text != "" {
			b.WriteString(segStyle(base, pending).Render(pending.text))
			pending.text = ""
		}
	}
	for _, s := range segs {
		if remaining <= 0 {
			break
		}
		text := sanitizeControls(s.text)
		if ansi.StringWidth(text) > remaining {
			text = ansi.Truncate(text, remaining, "")
		}
		if text == "" {
			continue
		}
		remaining -= ansi.StringWidth(text)
		// Merge runs that share a style so the row emits one SGR pair per color
		// change rather than one per segment.
		if pending.text != "" && pending.fg == s.fg && pending.bold == s.bold {
			pending.text += text
			continue
		}
		flush()
		pending = rowSegment{text: text, fg: s.fg, bold: s.bold}
	}
	flush()
	if remaining > 0 {
		b.WriteString(base.Render(strings.Repeat(" ", remaining)))
	}
	return b.String()
}

func formatItemRow(it listItem, selected bool, width int, t *treeState, label string, active bool) string {
	switch it.kind {
	case itemWindow:
		expanded := t.isWindowExpanded(it.session.Name, it.window.Index)
		return formatWindowRow(it.session.Name, it.window, expanded, selected, width, t, label, active)
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

	// Bold the session name on non-selected rows. The selected row gets its bold
	// treatment from the base style below, so leave the name plain there to avoid
	// double-wrapping. Pad the name with its own segment rather than %-18s because
	// the field width has to be measured in cells, not bytes.
	nameSeg := rowSegment{text: name}
	if !selected {
		nameSeg.fg = colorSessionName
		nameSeg.bold = true
	}
	pad := maxSessionNameDisplay - ansi.StringWidth(name)
	if pad < 0 {
		pad = 0
	}

	segs := []rowSegment{
		{text: fmt.Sprintf("%s %s ", chevron, status)},
		nameSeg,
		{text: strings.Repeat(" ", pad) + " " + ago},
	}

	rowWidth := width
	if icon, iconColor := commandIconPlain(s.ActiveCommand); iconColor != "" {
		segs = append(segs,
			rowSegment{text: " "},
			rowSegment{text: icon, fg: lipgloss.Color(iconColor)},
		)
		// Ambiguous-width icons (✦ etc.) render as 2 cells in most terminals but
		// ansi.StringWidth reports 1, so hand the row one cell back.
		rowWidth--
	}

	base := rowBaseStyle(selected, lipgloss.Color("#9CA3AF"))
	return renderRow(segs, base, rowWidth, label, false)
}

func formatWindowRow(sessionName string, w *tmux.Window, expanded, selected bool, width int, t *treeState, label string, active bool) string {
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
	// on non-selected rows — the selected row's cursor color takes over.
	nameSeg := rowSegment{text: w.Name}
	if isClaude && !selected {
		nameSeg.fg = colorClaude
	}

	segs := []rowSegment{
		{text: fmt.Sprintf("%s%s %s %d:", strings.Repeat(" ", indentWindow), chevron, marker, w.Index)},
		nameSeg,
	}
	if isClaude {
		segs = append(segs, claudeSuffix(info)...)
	}

	base := rowBaseStyle(selected, lipgloss.Color("#9CA3AF"))
	return renderRow(segs, base, width, label, active)
}

func formatPaneRow(p *tmux.Pane, selected bool, width int, t *treeState) string {
	marker := " "
	if p.Active {
		marker = "*"
	}

	segs := []rowSegment{
		{text: fmt.Sprintf("%s%s %d %s", strings.Repeat(" ", indentPane), marker, p.Index, p.Command)},
	}

	if info, ok := t.claudeInfo(p.PID); ok && info.State != tmux.ClaudeNone {
		segs = append(segs, claudeSuffix(info)...)
	}

	base := rowBaseStyle(selected, lipgloss.Color("#6B7280"))
	return renderRow(segs, base, width, "", false)
}

// claudeSuffix returns the segments for " <icon> <elapsed>  <recap>" on a Claude
// row. renderRow truncates the row to width, so recap is left intact here.
func claudeSuffix(info tmux.ClaudeInfo) []rowSegment {
	icon, color := claudeStateGlyph(info.State)
	segs := []rowSegment{
		{text: "  "},
		{text: icon, fg: color},
	}
	if !info.Since.IsZero() {
		segs = append(segs, rowSegment{text: " " + formatElapsed(time.Since(info.Since))})
	}
	recap := info.Recap
	if info.State == tmux.ClaudeWaiting {
		recap = "Needs your input"
	}
	if recap != "" {
		segs = append(segs,
			rowSegment{text: "  "},
			rowSegment{text: recap, fg: color},
		)
	}
	return segs
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
