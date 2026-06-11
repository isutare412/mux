package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/lunemis/mux/tmux"
)

func TestFormatElapsed(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{12 * time.Second, "0m12s"},
		{3*time.Minute + 8*time.Second, "3m08s"},
		{time.Hour + 54*time.Minute + 31*time.Second, "1h54m31s"},
	}
	for _, c := range cases {
		if got := formatElapsed(c.d); got != c.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestWindowClaudeRollup(t *testing.T) {
	panes := []tmux.Pane{{PID: 1}, {PID: 2}, {PID: 3}}
	cache := map[int]tmux.ClaudeInfo{
		1: {State: tmux.ClaudeIdle, Recap: "idle one"},
		2: {State: tmux.ClaudeWaiting, Recap: "needs perm"},
		3: {State: tmux.ClaudeWorking, Recap: "working"},
	}
	got, ok := windowClaudeRollup(panes, cache)
	if !ok {
		t.Fatal("expected a rollup")
	}
	if got.State != tmux.ClaudeWaiting {
		t.Errorf("rollup state = %d, want Waiting (highest priority)", got.State)
	}
}

func TestWindowClaudeRollupNone(t *testing.T) {
	panes := []tmux.Pane{{PID: 1}}
	cache := map[int]tmux.ClaudeInfo{} // no claude info
	if _, ok := windowClaudeRollup(panes, cache); ok {
		t.Error("expected no rollup when no claude panes")
	}
}

func TestClaudeStateGlyph(t *testing.T) {
	if icon, _ := claudeStateGlyph(tmux.ClaudeWaiting); icon != "◆" {
		t.Errorf("waiting icon = %q, want ◆", icon)
	}
	if icon, _ := claudeStateGlyph(tmux.ClaudeWorking); icon != "▸" {
		t.Errorf("working icon = %q, want ▸", icon)
	}
	if icon, _ := claudeStateGlyph(tmux.ClaudeIdle); icon != "✓" {
		t.Errorf("idle icon = %q, want ✓", icon)
	}
}

func TestTreeStateClaudeCache(t *testing.T) {
	st := newTreeState()
	if _, ok := st.claudeInfo(42); ok {
		t.Error("expected miss on empty cache")
	}
	st.claudeCache[42] = tmux.ClaudeInfo{State: tmux.ClaudeWorking, Recap: "x"}
	got, ok := st.claudeInfo(42)
	if !ok || got.Recap != "x" {
		t.Errorf("claudeInfo(42) = %+v, %v", got, ok)
	}
}

func TestFormatPaneRowWithClaude(t *testing.T) {
	st := newTreeState()
	p := &tmux.Pane{Index: 1, Command: "claude", PID: 7}
	st.claudeCache[7] = tmux.ClaudeInfo{
		State: tmux.ClaudeIdle,
		Recap: "DP-3733 done",
		Since: time.Now().Add(-3 * time.Minute),
	}
	row := formatPaneRow(p, false, 80, &st)
	if !strings.Contains(stripANSI(row), "DP-3733 done") {
		t.Errorf("pane row missing recap: %q", stripANSI(row))
	}
	if !strings.Contains(stripANSI(row), "✓") {
		t.Errorf("pane row missing idle glyph: %q", stripANSI(row))
	}
}

func TestFormatWindowRowRollup(t *testing.T) {
	st := newTreeState()
	st.panesCache[paneCacheKey{session: "sess", window: 0}] = []tmux.Pane{
		{Index: 0, Command: "zsh", PID: 1},
		{Index: 1, Command: "claude", PID: 2},
	}
	st.claudeCache[2] = tmux.ClaudeInfo{State: tmux.ClaudeWaiting, Recap: "needs perm"}
	w := &tmux.Window{Index: 0, Name: "win"}
	row := formatWindowRow("sess", w, false, false, 80, &st, "")
	if !strings.Contains(stripANSI(row), "Needs your input") {
		t.Errorf("window row missing waiting label: %q", stripANSI(row))
	}
}

func TestFormatPaneRowWaitingOverride(t *testing.T) {
	st := newTreeState()
	p := &tmux.Pane{Index: 1, Command: "claude", PID: 9}
	st.claudeCache[9] = tmux.ClaudeInfo{
		State: tmux.ClaudeWaiting,
		Recap: "colorizing the picker",
		Since: time.Now().Add(-2 * time.Minute),
	}
	row := stripANSI(formatPaneRow(p, false, 80, &st))
	if !strings.Contains(row, "Needs your input") {
		t.Errorf("waiting row missing override label: %q", row)
	}
	if strings.Contains(row, "colorizing the picker") {
		t.Errorf("waiting row should hide recap subject: %q", row)
	}
}

// stripANSI removes lipgloss color escape sequences for assertion.
func stripANSI(s string) string {
	return ansi.Strip(s)
}
