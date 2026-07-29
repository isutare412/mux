package ui

import (
	"testing"

	"github.com/lunemis/mux/tmux"
)

// navFixture builds a flattened list shaped like the real tree so the stop
// tests read as trees rather than index arithmetic.
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
