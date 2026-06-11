package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lunemis/mux/tmux"
)

// killRunner captures tmux Run invocations during confirm-kill tests.
type killRunner struct {
	runs []string
}

func (r *killRunner) Output(name string, args ...string) ([]byte, error) { return nil, nil }
func (r *killRunner) Run(name string, args ...string) error {
	r.runs = append(r.runs, name+" "+strings.Join(args, " "))
	return nil
}

// pressKey sends a single-rune key to the confirm model and returns the model
// and the message produced by its returned command (nil if no command).
func pressKey(m confirmKillModel, key string) (confirmKillModel, tea.Msg) {
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

func TestConfirmKill_Window_DispatchesKillWindow(t *testing.T) {
	rec := &killRunner{}
	tmux.SetRunner(rec)

	m := newConfirmKillModel(killTarget{kind: itemWindow, session: "dev", windowIndex: 2, label: "2:logs"})
	_, msg := pressKey(m, "y")

	km, ok := msg.(killedMsg)
	if !ok {
		t.Fatalf("expected killedMsg, got %T (%v)", msg, msg)
	}
	if !km.done || km.kind != itemWindow || km.session != "dev" {
		t.Errorf("killedMsg = %+v, want done window dev", km)
	}
	if len(rec.runs) != 1 || rec.runs[0] != "tmux kill-window -t dev:2" {
		t.Errorf("tmux calls = %v, want [tmux kill-window -t dev:2]", rec.runs)
	}
}

func TestConfirmKill_Pane_DispatchesKillPane(t *testing.T) {
	rec := &killRunner{}
	tmux.SetRunner(rec)

	m := newConfirmKillModel(killTarget{kind: itemPane, session: "dev", windowIndex: 1, paneIndex: 3, label: "3:zsh"})
	_, msg := pressKey(m, "y")

	km, ok := msg.(killedMsg)
	if !ok {
		t.Fatalf("expected killedMsg, got %T (%v)", msg, msg)
	}
	if !km.done || km.kind != itemPane || km.session != "dev" {
		t.Errorf("killedMsg = %+v, want done pane dev", km)
	}
	if len(rec.runs) != 1 || rec.runs[0] != "tmux kill-pane -t dev:1.3" {
		t.Errorf("tmux calls = %v, want [tmux kill-pane -t dev:1.3]", rec.runs)
	}
}

func TestConfirmKill_Session_DispatchesKillSession(t *testing.T) {
	rec := &killRunner{}
	tmux.SetRunner(rec)

	m := newConfirmKillModel(killTarget{kind: itemSession, session: "dev", label: "dev"})
	_, msg := pressKey(m, "y")

	km, ok := msg.(killedMsg)
	if !ok {
		t.Fatalf("expected killedMsg, got %T (%v)", msg, msg)
	}
	if !km.done || km.kind != itemSession || km.session != "dev" {
		t.Errorf("killedMsg = %+v, want done session dev", km)
	}
	if len(rec.runs) != 1 || rec.runs[0] != "tmux kill-session -t dev" {
		t.Errorf("tmux calls = %v, want [tmux kill-session -t dev]", rec.runs)
	}
}

func TestConfirmKill_Cancel(t *testing.T) {
	rec := &killRunner{}
	tmux.SetRunner(rec)

	m := newConfirmKillModel(killTarget{kind: itemWindow, session: "dev", windowIndex: 2, label: "2:logs"})
	_, msg := pressKey(m, "n")

	km, ok := msg.(killedMsg)
	if !ok {
		t.Fatalf("expected killedMsg, got %T (%v)", msg, msg)
	}
	if km.done {
		t.Errorf("killedMsg.done = true on cancel, want false")
	}
	if len(rec.runs) != 0 {
		t.Errorf("tmux calls = %v, want none on cancel", rec.runs)
	}
}

func TestConfirmKill_ViewLabelsWindow(t *testing.T) {
	m := newConfirmKillModel(killTarget{kind: itemWindow, session: "dev", windowIndex: 2, label: "2:logs"})
	if got := m.View(); !strings.Contains(got, `Kill window "2:logs"?`) {
		t.Errorf("View() = %q, want it to contain `Kill window \"2:logs\"?`", got)
	}
}
