package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestNavigationCancelsPendingFocus(t *testing.T) {
	m := NewModel()
	sessions := []tmux.Session{{Name: "mux"}, {Name: "eval"}}

	m = drive(m, sessionsLoadedMsg{sessions: sessions})
	m = drive(m, currentContextMsg{session: "mux", window: 1, ok: true})

	// User navigates before mux's windows load.
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	movedCursor := m.cursor

	// Windows now arrive; the cursor must NOT be yanked back to mux's window.
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim"},
		{Index: 1, Name: "claude", Active: true},
	}})

	if m.focusSession != "" {
		t.Errorf("focusSession = %q, want empty after navigation cancels pending focus", m.focusSession)
	}
	// After windows load, the item list expands to:
	//   [0] mux (session), [1] nvim (window:0), [2] claude (window:1), [3] eval (session)
	// The cursor was at position 1 (eval) before the load, and must stay under
	// the item the user moved to — not snapped to mux's active window (index 1).
	if it := m.currentItem(); it != nil && it.kind == itemWindow && it.session.Name == "mux" && it.window.Index == 1 {
		t.Errorf("cursor was yanked to mux:1 after user navigated; cursor=%d", m.cursor)
	}
	_ = movedCursor
}

func TestPostCreateFocusLandsOnSessionRow(t *testing.T) {
	m := NewModel()
	// User creates a new session "fresh".
	m = drive(m, sessionCreatedMsg{name: "fresh"})

	sessions := []tmux.Session{{Name: "mux"}, {Name: "fresh"}}
	m = drive(m, sessionsLoadedMsg{sessions: sessions})

	it := m.currentItem()
	if it == nil || it.kind != itemSession || it.session.Name != "fresh" {
		t.Fatalf("cursor not on session 'fresh'; got %+v", it)
	}
}
