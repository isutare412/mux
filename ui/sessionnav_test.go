package ui

import (
	"testing"

	"github.com/lunemis/mux/tmux"
)

// navSession describes a test session with its windows and panes, used to
// construct a flattened list for testing.
type navSession struct {
	name     string
	windows  []tmux.Window
	panesFor map[int][]tmux.Pane // window index -> pane rows to emit under it
}

func buildNavItems(sessions []navSession) []listItem {
	// Pointers must outlive the loop, so allocate backing arrays up front.
	sess := make([]tmux.Session, len(sessions))
	items := []listItem{}
	for i := range sessions {
		sess[i] = tmux.Session{Name: sessions[i].name}
		items = append(items, listItem{kind: itemSession, session: &sess[i]})

		wins := make([]tmux.Window, len(sessions[i].windows))
		copy(wins, sessions[i].windows)
		for j := range wins {
			items = append(items, listItem{kind: itemWindow, session: &sess[i], window: &wins[j]})

			panes := sessions[i].panesFor[wins[j].Index]
			ps := make([]tmux.Pane, len(panes))
			copy(ps, panes)
			for k := range ps {
				items = append(items, listItem{
					kind: itemPane, session: &sess[i], window: &wins[j], pane: &ps[k],
				})
			}
		}
	}
	return items
}

func assertStops(t *testing.T, items []listItem, want []int) {
	t.Helper()
	got := sessionStops(items)
	if len(got) != len(want) {
		t.Fatalf("stops = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stops[%d] = %d, want %d (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestSessionStops_StarredWindowPerSession(t *testing.T) {
	items := buildNavItems([]navSession{
		{name: "dotfiles", windows: []tmux.Window{
			{Index: 1, Name: "nvim"},
			{Index: 2, Name: "claude", Active: true},
		}},
		{name: "common", windows: []tmux.Window{
			{Index: 1, Name: "nvim", Active: true},
		}},
	})
	// [0]dotfiles [1]w1 [2]w2* [3]common [4]w1*
	assertStops(t, items, []int{2, 4})
}

// A session contributing no window rows falls back to its own row. This covers
// both spec cases at once — collapsed, and expanded but windows not yet loaded
// — because sessionStops sees only the flattened items, where the two are
// indistinguishable.
func TestSessionStops_CollapsedSessionUsesItsOwnRow(t *testing.T) {
	items := buildNavItems([]navSession{
		{name: "dotfiles", windows: []tmux.Window{{Index: 1, Active: true}}},
		{name: "collapsed"}, // no window rows emitted
		{name: "common", windows: []tmux.Window{{Index: 1, Active: true}}},
	})
	// [0]dotfiles [1]w1* [2]collapsed [3]common [4]w1*
	assertStops(t, items, []int{1, 2, 4})
}

func TestSessionStops_NoActiveWindowFallsBackToSessionRow(t *testing.T) {
	items := buildNavItems([]navSession{
		{name: "weird", windows: []tmux.Window{{Index: 1}, {Index: 2}}},
	})
	// [0]weird [1]w1 [2]w2 — none active
	assertStops(t, items, []int{0})
}

func TestSessionStops_PaneRowsAreNeverStops(t *testing.T) {
	items := buildNavItems([]navSession{
		{
			name:    "dotfiles",
			windows: []tmux.Window{{Index: 1}, {Index: 2, Active: true}},
			// An ACTIVE pane under the non-active window 1: the star in the
			// pane column must not attract J/K.
			panesFor: map[int][]tmux.Pane{1: {{Index: 0, Active: true}}},
		},
	})
	// [0]dotfiles [1]w1 [2]pane* [3]w2*
	assertStops(t, items, []int{3})
}

func TestSessionStops_Empty(t *testing.T) {
	assertStops(t, nil, nil)
}

// navModel builds a Model with two sessions whose windows are loaded, so the
// item list is [mux, mux:0*, mux:1, dotfiles, dotfiles:1, dotfiles:2*].
func navModel(t *testing.T) Model {
	t.Helper()
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "dotfiles"}}})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim", Active: true},
		{Index: 1, Name: "zsh"},
	}})
	m = drive(m, windowsLoadedMsg{sessionName: "dotfiles", windows: []tmux.Window{
		{Index: 1, Name: "nvim"},
		{Index: 2, Name: "claude", Active: true},
	}})
	return m
}

// stopIndex returns the index of the given session's stop, so assertions read
// by session name instead of by a hard-coded row number.
func stopIndex(t *testing.T, m Model, session string) int {
	t.Helper()
	for _, s := range sessionStops(m.items) {
		if m.items[s].session.Name == session {
			return s
		}
	}
	t.Fatalf("no stop for session %q", session)
	return -1
}

func TestNextSessionStop_ForwardAndBackward(t *testing.T) {
	m := navModel(t)
	first := sessionStops(m.items)[0]
	second := sessionStops(m.items)[1]

	if got, ok := nextSessionStop(m.items, first, 1); !ok || got != second {
		t.Errorf("forward from %d = (%d, %v), want (%d, true)", first, got, ok, second)
	}
	if got, ok := nextSessionStop(m.items, second, -1); !ok || got != first {
		t.Errorf("backward from %d = (%d, %v), want (%d, true)", second, got, ok, first)
	}
}

func TestJumpKeysMoveBetweenSessions(t *testing.T) {
	m := navModel(t)
	m.cursor = 0 // the mux session row

	m = drive(m, runeKey('J'))
	if want := stopIndex(t, m, "mux"); m.cursor != want {
		t.Fatalf("J from the top: cursor = %d, want %d (mux stop)", m.cursor, want)
	}

	m = drive(m, runeKey('J'))
	if want := stopIndex(t, m, "dotfiles"); m.cursor != want {
		t.Fatalf("second J: cursor = %d, want %d (dotfiles stop)", m.cursor, want)
	}

	m = drive(m, runeKey('K'))
	if want := stopIndex(t, m, "mux"); m.cursor != want {
		t.Fatalf("K: cursor = %d, want %d (mux stop)", m.cursor, want)
	}
}

// From a row that is not itself a stop, J/K move to the adjacent stop.
func TestJumpKeysFromNonStopRow(t *testing.T) {
	m := navModel(t)
	m.cursor = stopIndex(t, m, "mux") + 1 // mux:1, not the starred row

	m = drive(m, runeKey('J'))
	if want := stopIndex(t, m, "dotfiles"); m.cursor != want {
		t.Errorf("J: cursor = %d, want %d", m.cursor, want)
	}

	m.cursor = stopIndex(t, m, "mux") + 1
	m = drive(m, runeKey('K'))
	if want := stopIndex(t, m, "mux"); m.cursor != want {
		t.Errorf("K: cursor = %d, want %d", m.cursor, want)
	}
}

func TestJumpKeysClampAtTheEnds(t *testing.T) {
	m := navModel(t)

	m.cursor = stopIndex(t, m, "dotfiles")
	last := m.cursor
	m = drive(m, runeKey('J'))
	if m.cursor != last {
		t.Errorf("J at the final stop moved: %d -> %d", last, m.cursor)
	}

	m.cursor = stopIndex(t, m, "mux")
	first := m.cursor
	m = drive(m, runeKey('K'))
	if m.cursor != first {
		t.Errorf("K at the first stop moved: %d -> %d", first, m.cursor)
	}
}

func TestJumpKeysClearPendingFocus(t *testing.T) {
	m := navModel(t)
	m.focusSession = "dotfiles"
	m.focusWindow = 2

	m = drive(m, runeKey('J'))

	if m.focusSession != "" {
		t.Errorf("focusSession = %q, want empty after J", m.focusSession)
	}
	if m.focusWindow != -1 {
		t.Errorf("focusWindow = %d, want -1 after J", m.focusWindow)
	}
}
