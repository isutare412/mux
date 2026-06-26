package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/lunemis/mux/tmux"
)

// TestMain forces a truecolor profile so lipgloss emits ANSI escape codes even
// without a TTY, making the color/bold assertions below deterministic.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(0) // termenv.TrueColor
	os.Exit(m.Run())
}

func TestFormatSessionRow_NameBoldWhenNotSelected(t *testing.T) {
	s := tmux.Session{Name: "mux"}
	row := formatSessionRow(s, false, false, 60, "")
	want := lipgloss.NewStyle().Bold(true).Foreground(colorSessionName).Render("mux")
	if !strings.Contains(row, want) {
		t.Errorf("expected bold yellow session name %q in %q", want, row)
	}
}

func TestFormatSessionRow_NameNotBoldWhenSelected(t *testing.T) {
	s := tmux.Session{Name: "mux"}
	row := formatSessionRow(s, false, true, 60, "")
	standaloneBold := lipgloss.NewStyle().Bold(true).Render("mux")
	if strings.Contains(row, standaloneBold) {
		t.Errorf("did not expect standalone bold name in selected row %q", row)
	}
	if !strings.Contains(ansi.Strip(row), "mux") {
		t.Errorf("expected name present in selected row %q", row)
	}
}

func TestFormatSessionRow_OmitsGitBranch(t *testing.T) {
	s := tmux.Session{Name: "mux", GitBranch: "feature/DP-3774-healthcheck"}
	row := formatSessionRow(s, false, false, 80, "")
	if strings.Contains(row, "feature/DP-3774-healthcheck") {
		t.Errorf("expected git branch to be omitted from session row, got %q", row)
	}
}

func TestFormatSessionRow_BoldPreservesWidth(t *testing.T) {
	s := tmux.Session{Name: "mux"}
	plain := formatSessionRow(s, false, true, 60, "") // selected: plain name
	bold := formatSessionRow(s, false, false, 60, "") // non-selected: bold name
	if ansi.StringWidth(plain) != ansi.StringWidth(bold) {
		t.Errorf("bold changed row width: %d vs %d", ansi.StringWidth(plain), ansi.StringWidth(bold))
	}
}

func TestFormatWindowRow_LongNameNotElided(t *testing.T) {
	st := newTreeState()
	w := &tmux.Window{Index: 0, Name: "claude:eval-platform"}
	row := formatWindowRow("sess", w, false, false, 60, &st, "", false)
	if !strings.Contains(row, "claude:eval-platform") {
		t.Errorf("expected full window name in %q", row)
	}
	if strings.Contains(row, "...") {
		t.Errorf("expected no ellipsis in %q", row)
	}
}

func TestFormatPaneRow_LongCommandNotElided(t *testing.T) {
	st := newTreeState()
	p := &tmux.Pane{Index: 0, Command: "node /usr/local/bin/some-long-command"}
	row := formatPaneRow(p, false, 60, &st)
	if !strings.Contains(row, "node /usr/local/bin/some-long-command") {
		t.Errorf("expected full pane command in %q", row)
	}
	if strings.Contains(row, "...") {
		t.Errorf("expected no ellipsis in %q", row)
	}
}

func TestFormatSessionRow_JumpLabelReplacesChevron(t *testing.T) {
	s := tmux.Session{Name: "mux"}
	plain := formatSessionRow(s, true, false, 60, "")
	row := formatSessionRow(s, true, false, 60, "q") // "q" is not in "mux"
	if !strings.Contains(row, "q") {
		t.Errorf("expected label \"q\" in %q", row)
	}
	if strings.Contains(row, "▼") {
		t.Errorf("expected chevron replaced by label in %q", row)
	}
	if ansi.StringWidth(plain) != ansi.StringWidth(row) {
		t.Errorf("row width changed with label: %d vs %d", ansi.StringWidth(plain), ansi.StringWidth(row))
	}
}

func TestFormatSessionRow_NoLabelKeepsChevron(t *testing.T) {
	s := tmux.Session{Name: "mux"}
	row := formatSessionRow(s, true, false, 60, "")
	if !strings.Contains(row, "▼") {
		t.Errorf("expected chevron present when no label in %q", row)
	}
}

func TestFormatWindowRow_JumpLabelKeepsChevronNoShift(t *testing.T) {
	st := newTreeState()
	w := &tmux.Window{Index: 0, Name: "nvim"} // no "q" in the name
	plain := formatWindowRow("sess", w, false, false, 60, &st, "", false)
	labeled := formatWindowRow("sess", w, false, false, 60, &st, "q", true)

	if !strings.Contains(labeled, "q") {
		t.Errorf("expected label \"q\" in %q", labeled)
	}
	if !strings.Contains(labeled, "▶") {
		t.Errorf("window chevron should remain (label replaces indent, not chevron): %q", labeled)
	}
	if ansi.StringWidth(plain) != ansi.StringWidth(labeled) {
		t.Errorf("row width changed with label: %d vs %d", ansi.StringWidth(plain), ansi.StringWidth(labeled))
	}
}

func TestFormatWindowRow_ClaudeNameOrangeWhenNotSelected(t *testing.T) {
	st := newTreeState()
	st.panesCache[paneCacheKey{session: "sess", window: 0}] = []tmux.Pane{{Index: 0, PID: 2}}
	st.claudeCache[2] = tmux.ClaudeInfo{State: tmux.ClaudeWaiting}
	w := &tmux.Window{Index: 0, Name: "claude"}
	row := formatWindowRow("sess", w, false, false, 60, &st, "", false)
	wantOrange := lipgloss.NewStyle().Foreground(colorClaude).Render("claude")
	if !strings.Contains(row, wantOrange) {
		t.Errorf("expected orange claude window name %q in %q", wantOrange, row)
	}
}

func TestFormatWindowRow_NonClaudeNameNotOrange(t *testing.T) {
	st := newTreeState()
	w := &tmux.Window{Index: 0, Name: "nvim"}
	row := formatWindowRow("sess", w, false, false, 60, &st, "", false)
	wantOrange := lipgloss.NewStyle().Foreground(colorClaude).Render("nvim")
	if strings.Contains(row, wantOrange) {
		t.Errorf("did not expect orange styling on non-claude window %q", row)
	}
}

func TestFormatWindowRow_ClaudeNamePlainWhenSelected(t *testing.T) {
	st := newTreeState()
	st.panesCache[paneCacheKey{session: "sess", window: 0}] = []tmux.Pane{{Index: 0, PID: 2}}
	st.claudeCache[2] = tmux.ClaudeInfo{State: tmux.ClaudeWaiting}
	w := &tmux.Window{Index: 0, Name: "claude"}
	row := formatWindowRow("sess", w, false, true, 60, &st, "", false)
	wantOrange := lipgloss.NewStyle().Foreground(colorClaude).Render("claude")
	if strings.Contains(row, wantOrange) {
		t.Errorf("did not expect orange name on selected row %q", row)
	}
	if !strings.Contains(ansi.Strip(row), "claude") {
		t.Errorf("expected name present in selected row %q", row)
	}
}

func TestFormatWindowRow_OrangePreservesWidth(t *testing.T) {
	st := newTreeState()
	st.panesCache[paneCacheKey{session: "sess", window: 0}] = []tmux.Pane{{Index: 0, PID: 2}}
	st.claudeCache[2] = tmux.ClaudeInfo{State: tmux.ClaudeWaiting}
	w := &tmux.Window{Index: 0, Name: "claude"}
	plain := formatWindowRow("sess", w, false, true, 60, &st, "", false)   // selected: plain name
	orange := formatWindowRow("sess", w, false, false, 60, &st, "", false) // non-selected: orange name
	if ansi.StringWidth(plain) != ansi.StringWidth(orange) {
		t.Errorf("orange changed row width: %d vs %d", ansi.StringWidth(plain), ansi.StringWidth(orange))
	}
}

// styleOpen returns the leading SGR sequence lipgloss emits for style s, by
// probing it against a sentinel rune and slicing off everything before it.
func styleOpen(s lipgloss.Style) string {
	probe := s.Render("X")
	return probe[:strings.Index(probe, "X")]
}

func TestStyleRow_LabelColorByActive(t *testing.T) {
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))

	inactive := styleRow("  text", base, "q", false)
	wantMuted := styleOpen(base.Foreground(colorMuted))
	if !strings.HasPrefix(inactive, wantMuted) {
		t.Errorf("inactive label should open with muted color %q, got %q", wantMuted, inactive)
	}

	active := styleRow("  text", base, "q", true)
	wantAccent := styleOpen(base.Bold(true).Foreground(colorAccent))
	if !strings.HasPrefix(active, wantAccent) {
		t.Errorf("active label should open with bold accent %q, got %q", wantAccent, active)
	}
}

func TestFormatSessionRow_JumpLabelDoesNotLeakIntoName(t *testing.T) {
	s := tmux.Session{Name: "mux"}
	row := formatSessionRow(s, false, false, 60, "q") // "q" is not in "mux"

	grayOpen := styleOpen(lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")))
	labelIdx := strings.Index(row, "q")
	if labelIdx < 0 {
		t.Fatalf("label \"q\" not found in %q", row)
	}
	if !strings.Contains(row[labelIdx:], grayOpen) {
		t.Errorf("base gray not re-asserted after jump label; name washes out: %q", row)
	}
}

func TestFormatSessionRow_JumpLabelKeepsCursorHighlight(t *testing.T) {
	s := tmux.Session{Name: "mux"}
	row := formatSessionRow(s, false, true, 60, "q") // selected + jump label

	// lipgloss collapses bold+fg+bg into a single SGR sequence, so probe the full
	// base style and look for its background parameter rather than a standalone
	// background open sequence (which never appears verbatim in the combined form).
	bgOpen := styleOpen(lipgloss.NewStyle().Background(colorSelected))
	bgParam := strings.TrimSuffix(strings.TrimPrefix(bgOpen, "\x1b["), "m")
	labelIdx := strings.Index(row, "q")
	if labelIdx < 0 {
		t.Fatalf("label \"q\" not found in %q", row)
	}
	if !strings.Contains(row[labelIdx:], bgParam) {
		t.Errorf("selected background not present after jump label; highlight bar breaks: %q", row)
	}
}

func TestRenderListView_LabelsVisibleWithoutJumpMode(t *testing.T) {
	sess := tmux.Session{Name: "mux"}
	w := tmux.Window{Index: 0, Name: "nvim"}
	items := []listItem{
		{kind: itemSession, session: &sess},
		{kind: itemWindow, session: &sess, window: &w},
	}
	labels := []string{"", "a"} // window row gets label "a"
	st := newTreeState()

	// Not in jump mode: the window label must still be present, drawn muted.
	resting := renderListView(items, 0, "", &st, 60, 10, labels, false)
	mutedOpen := styleOpen(lipgloss.NewStyle().Foreground(colorMuted))
	if !strings.Contains(resting, mutedOpen) {
		t.Errorf("resting label should be visible in muted color: %q", resting)
	}

	// In jump mode: the same label flips to the bold accent color.
	active := renderListView(items, 0, "", &st, 60, 10, labels, true)
	accentOpen := styleOpen(lipgloss.NewStyle().Bold(true).Foreground(colorAccent))
	if !strings.Contains(active, accentOpen) {
		t.Errorf("active label should be drawn in bold accent: %q", active)
	}
}
