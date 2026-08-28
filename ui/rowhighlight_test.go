package ui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lunemis/mux/tmux"
)

// sgrBackground folds one SGR parameter string into the background state it
// leaves behind, returning "" when the sequence clears the background. Only the
// forms lipgloss emits are handled: a plain reset, 49, and truecolor 48;2;r;g;b.
func sgrBackground(prev, params string) string {
	if params == "" {
		return ""
	}
	toks := strings.Split(params, ";")
	bg := prev
	for i := 0; i < len(toks); i++ {
		switch toks[i] {
		case "", "0":
			bg = ""
		case "49":
			bg = ""
		case "48":
			if i+4 < len(toks) && toks[i+1] == "2" {
				bg = strings.Join(toks[i:i+5], ";")
				i += 4
			}
		case "38":
			if i+4 < len(toks) && toks[i+1] == "2" {
				i += 4
			}
		}
	}
	return bg
}

// rowCellBackgrounds walks a rendered row and reports the background in effect
// at every visible cell, so a test can prove the highlight bar has no gaps.
func rowCellBackgrounds(row string) []string {
	var cells []string
	bg := ""
	for i := 0; i < len(row); {
		if row[i] == 0x1b && i+1 < len(row) && row[i+1] == '[' {
			j := i + 2
			for j < len(row) && row[j] != 'm' {
				j++
			}
			if j >= len(row) {
				break
			}
			bg = sgrBackground(bg, row[i+2:j])
			i = j + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(row[i:])
		for w := ansi.StringWidth(string(r)); w > 0; w-- {
			cells = append(cells, bg)
		}
		i += size
	}
	return cells
}

// selectedBGParam is the truecolor background parameter of the cursor highlight.
func selectedBGParam(t *testing.T) string {
	t.Helper()
	open := styleOpen(lipgloss.NewStyle().Background(colorSelected))
	return strings.TrimSuffix(strings.TrimPrefix(open, "\x1b["), "m")
}

// assertHighlightSpansWidth fails unless every one of the row's width cells
// carries the selection background.
func assertHighlightSpansWidth(t *testing.T, row string, width int) {
	t.Helper()
	want := selectedBGParam(t)
	cells := rowCellBackgrounds(row)
	if len(cells) != width {
		t.Fatalf("row covers %d cells, want %d: %q", len(cells), width, row)
	}
	for i, bg := range cells {
		if bg != want {
			t.Fatalf("cell %d has background %q, want %q: %q", i, bg, want, row)
		}
	}
}

func claudeWindowState(state tmux.ClaudeState, recap string) treeState {
	st := newTreeState()
	st.panesCache[paneCacheKey{session: "sess", window: 2}] = []tmux.Pane{{Index: 0, PID: 7}}
	st.claudeCache[7] = tmux.ClaudeInfo{
		State: state,
		Since: time.Now().Add(-12 * time.Second),
		Recap: recap,
	}
	return st
}

func TestFormatWindowRow_SelectedHighlightSpansFullWidthWithClaudeBadge(t *testing.T) {
	st := claudeWindowState(tmux.ClaudeIdle, "")
	w := &tmux.Window{Index: 2, Name: "claude"}
	row := formatWindowRow("sess", w, false, true, 60, &st, "", false)
	assertHighlightSpansWidth(t, row, 60)
}

func TestFormatWindowRow_SelectedHighlightSpansFullWidthWithRecap(t *testing.T) {
	st := claudeWindowState(tmux.ClaudeWorking, "top border rendering")
	w := &tmux.Window{Index: 2, Name: "claude"}
	row := formatWindowRow("sess", w, false, true, 60, &st, "", false)
	assertHighlightSpansWidth(t, row, 60)
}

func TestFormatWindowRow_SelectedHighlightSpansFullWidthWithJumpLabel(t *testing.T) {
	st := claudeWindowState(tmux.ClaudeIdle, "")
	w := &tmux.Window{Index: 2, Name: "claude"}
	row := formatWindowRow("sess", w, false, true, 60, &st, "s", true)
	assertHighlightSpansWidth(t, row, 60)
}

func TestFormatWindowRow_SelectedHighlightSpansFullWidthWithoutClaude(t *testing.T) {
	st := newTreeState()
	w := &tmux.Window{Index: 3, Name: "zsh"}
	row := formatWindowRow("sess", w, false, true, 60, &st, "d", false)
	assertHighlightSpansWidth(t, row, 60)
}

func TestFormatSessionRow_SelectedHighlightSpansFullWidthWithAIIcon(t *testing.T) {
	s := tmux.Session{Name: "mux", ActiveCommand: "claude"}
	row := formatSessionRow(s, false, true, 60, "a")
	// The AI icon is an ambiguous-width rune: two cells on the terminal, one by
	// ansi.StringWidth. The row gives a cell back for it, so it measures 59 here.
	assertHighlightSpansWidth(t, row, 59)
}

func TestFormatPaneRow_SelectedHighlightSpansFullWidthWithClaudeBadge(t *testing.T) {
	st := newTreeState()
	st.claudeCache[7] = tmux.ClaudeInfo{
		State: tmux.ClaudeWaiting,
		Since: time.Now().Add(-90 * time.Second),
	}
	p := &tmux.Pane{Index: 0, PID: 7, Command: "node"}
	row := formatPaneRow(p, true, 60, &st)
	assertHighlightSpansWidth(t, row, 60)
}

func TestFormatWindowRow_ClaudeBadgeKeepsStateColorWhenSelected(t *testing.T) {
	st := claudeWindowState(tmux.ClaudeIdle, "")
	w := &tmux.Window{Index: 2, Name: "claude"}
	row := formatWindowRow("sess", w, false, true, 60, &st, "", false)

	base := rowBaseStyle(true, lipgloss.Color("#9CA3AF"))
	wantOpen := styleOpen(base.Foreground(colorClaudeIdle))
	if !strings.Contains(row, wantOpen) {
		t.Errorf("expected badge to keep its state color over the highlight (%q) in %q", wantOpen, row)
	}
}
