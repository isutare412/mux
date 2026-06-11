package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/lunemis/mux/tmux"
)

func TestFormatWindowRow_LongNameNotElided(t *testing.T) {
	st := newTreeState()
	w := &tmux.Window{Index: 0, Name: "claude:eval-platform"}
	row := formatWindowRow("sess", w, false, false, 60, &st, "")
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
	plain := formatWindowRow("sess", w, false, false, 60, &st, "")
	labeled := formatWindowRow("sess", w, false, false, 60, &st, "q")

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
