package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lunemis/mux/tmux"
)

// pressRune sends a single-rune key to the model and returns the updated Model.
func pressRune(m Model, r string) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)})
	return next.(Model)
}

func TestKillKey_OnWindow_OpensConfirmWithWindowTarget(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}}})
	m = drive(m, currentContextMsg{session: "mux", window: 1, ok: true})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim"},
		{Index: 1, Name: "claude", Active: true},
	}})

	// Cursor is on window 1 (the active window). Press x.
	m = pressRune(m, "x")

	if m.mode != modeConfirmKill {
		t.Fatalf("mode = %v, want modeConfirmKill", m.mode)
	}
	tgt := m.confirmKillMod.target
	if tgt.kind != itemWindow || tgt.session != "mux" || tgt.windowIndex != 1 {
		t.Errorf("target = %+v, want window mux:1", tgt)
	}
}
