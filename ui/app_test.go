package ui

import (
	"testing"

	"github.com/lunemis/mux/tmux"
)

// drive applies a message and returns the updated Model.
func drive(m Model, msg any) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestStartupFocusLandsOnActiveWindow(t *testing.T) {
	m := NewModel()
	sessions := []tmux.Session{{Name: "mux"}, {Name: "eval"}}

	m = drive(m, sessionsLoadedMsg{sessions: sessions})
	m = drive(m, currentContextMsg{session: "mux", window: 1, ok: true})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim"},
		{Index: 1, Name: "claude", Active: true},
	}})

	it := m.currentItem()
	if it == nil || it.kind != itemWindow || it.window.Index != 1 {
		t.Fatalf("cursor not on window 1; got %+v", it)
	}
}

func TestStartupFocusSurvivesTick(t *testing.T) {
	m := NewModel()
	sessions := []tmux.Session{{Name: "mux"}, {Name: "eval"}}

	m = drive(m, sessionsLoadedMsg{sessions: sessions})
	m = drive(m, currentContextMsg{session: "mux", window: 1, ok: true})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim"},
		{Index: 1, Name: "claude", Active: true},
	}})

	// A later session-list refresh (tick) must not reset the cursor.
	m = drive(m, sessionsLoadedMsg{sessions: sessions})

	it := m.currentItem()
	if it == nil || it.kind != itemWindow || it.window.Index != 1 {
		t.Fatalf("cursor moved off window 1 after refresh; got %+v", it)
	}
}

func TestStartupFocusIgnoredWhenNotOk(t *testing.T) {
	m := NewModel()
	sessions := []tmux.Session{{Name: "mux"}, {Name: "eval"}}

	m = drive(m, sessionsLoadedMsg{sessions: sessions})
	m.cursor = 1 // user is somewhere other than the top

	m = drive(m, currentContextMsg{ok: false})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim", Active: true},
	}})

	if m.focusSession != "" {
		t.Errorf("focusSession = %q, want empty (no focus set when ok=false)", m.focusSession)
	}
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (unchanged when context not ok)", m.cursor)
	}
}
