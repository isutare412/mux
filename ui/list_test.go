package ui

import (
	"strings"
	"testing"

	"github.com/lunemis/mux/tmux"
)

func TestFormatWindowRow_LongNameNotElided(t *testing.T) {
	st := newTreeState()
	w := &tmux.Window{Index: 0, Name: "claude:eval-platform"}
	row := formatWindowRow("sess", w, false, false, 60, &st)
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
