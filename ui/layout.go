package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// isControl reports whether r is a control character other than ESC. ESC is
// spared because it opens the SGR sequences lipgloss emits for color.
func isControl(r rune) bool {
	return (r < 0x20 && r != 0x1b) || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// sanitizeControls replaces control characters with spaces.
//
// Every row and every panel line reaches the screen through padOrTruncate, and
// its contract is a fixed cell count. Control characters break that contract
// silently: ansi.StringWidth — and bubbletea's own line diff, which uses the
// same measure — scores them as zero cells while the terminal acts on them. A
// tab advances to the next tab stop and a newline opens a row, so the frame
// outgrows the terminal and the alt buffer scrolls; a carriage return jumps to
// column 0 and the rest of the line repaints over the panel beside it. Either
// way the renderer's cached lines stop matching the screen and stale rows from
// earlier frames stay put. The text that carries them is not ours — Claude
// recaps, tmux window names, capture-pane output — so neutralize it here
// rather than at each call site.
func sanitizeControls(s string) string {
	if strings.IndexFunc(s, isControl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return ' '
		}
		return r
	}, s)
}

// padOrTruncate ensures a string is exactly `width` visible characters
func padOrTruncate(s string, width int) string {
	s = sanitizeControls(s)
	w := ansi.StringWidth(s)
	if w > width {
		return ansi.Truncate(s, width, "")
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// fixedBox takes rendered content and forces it to exactly width x height visible area.
// It splits by newlines, truncates/pads each line to width, and truncates/pads to height lines.
func fixedBox(content string, width, height int) string {
	lines := strings.Split(content, "\n")

	result := make([]string, height)
	for i := 0; i < height; i++ {
		if i < len(lines) {
			result[i] = padOrTruncate(lines[i], width)
		} else {
			result[i] = strings.Repeat(" ", width)
		}
	}
	return strings.Join(result, "\n")
}

// joinHorizontalFixed joins two blocks of text side-by-side, line by line
func joinHorizontalFixed(left, right string) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	maxLen := len(leftLines)
	if len(rightLines) > maxLen {
		maxLen = len(rightLines)
	}

	result := make([]string, maxLen)
	for i := 0; i < maxLen; i++ {
		l := ""
		r := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		result[i] = l + r
	}
	return strings.Join(result, "\n")
}

// drawBorder wraps content lines with a rounded border.
//
// Only the border glyphs are colored. The content is emitted byte-for-byte
// between two independently styled bars, because it is raw capture-pane output:
// text the pane drew in the terminal's default color carries no ANSI of its
// own, so styling the assembled block would repaint it in the border color.
// Each bar closes its own color span, which also stops an unterminated sequence
// inside the content from bleeding past the frame.
func drawBorder(content string, width, height int) string {
	innerWidth := width - 2
	lines := strings.Split(content, "\n")

	borderStyle := lipgloss.NewStyle().Foreground(colorBorder)
	bar := borderStyle.Render("│")

	// Build bordered output
	result := make([]string, 0, height+2)

	// Top border
	result = append(result, borderStyle.Render("╭"+strings.Repeat("─", innerWidth)+"╮"))

	// Content lines (pad/truncate to exactly height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		line = padOrTruncate(line, innerWidth)
		result = append(result, bar+line+bar)
	}

	// Bottom border
	result = append(result, borderStyle.Render("╰"+strings.Repeat("─", innerWidth)+"╯"))

	return strings.Join(result, "\n")
}
