package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lunemis/mux/tmux"
	"github.com/muesli/termenv"
)

func TestAssignLabels_LabelsWindowsOnly(t *testing.T) {
	items := []listItem{
		{kind: itemSession},
		{kind: itemWindow},
		{kind: itemPane},
		{kind: itemWindow},
	}
	got := assignLabels(items, rowRef{}, rowRef{})
	want := []string{"", "a", "", "d"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("labels[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAssignLabels_Overflow(t *testing.T) {
	n := len(jumpAlphabet) + 2
	items := make([]listItem, n)
	for i := range items {
		items[i] = listItem{kind: itemWindow}
	}
	got := assignLabels(items, rowRef{}, rowRef{})
	if got[len(jumpAlphabet)-1] == "" {
		t.Errorf("last in-range row should have a label")
	}
	if got[len(jumpAlphabet)] != "" {
		t.Errorf("overflow row should have empty label, got %q", got[len(jumpAlphabet)])
	}
}

func TestRebuildItemsAssignsLabels(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}}})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim"},
		{Index: 1, Name: "claude"},
	}})

	if len(m.labels) != len(m.items) {
		t.Fatalf("labels len %d != items len %d", len(m.labels), len(m.items))
	}
	// items: [session mux, window 0, window 1] -> labels: ["", "a", "d"]
	if m.items[0].kind != itemSession || m.labels[0] != "" {
		t.Errorf("session row should be unlabelled, got kind=%v label=%q", m.items[0].kind, m.labels[0])
	}
	if m.items[1].kind != itemWindow || m.labels[1] != "a" {
		t.Errorf("first window label = %q, want \"a\"", m.labels[1])
	}
	if m.items[2].kind != itemWindow || m.labels[2] != "d" {
		t.Errorf("second window label = %q, want \"d\"", m.labels[2])
	}
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestJumpModeEnterAndJump(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}}})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim"},
		{Index: 1, Name: "claude"},
	}})
	// items: [session mux, window 0, window 1] -> labels: ["", "a", "d"]

	m = drive(m, runeKey('s')) // enter jump mode
	if m.mode != modeJump {
		t.Fatalf("mode = %v, want modeJump", m.mode)
	}

	m = drive(m, runeKey('d')) // 'd' is the label for window 1
	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList after jump", m.mode)
	}
	it := m.currentItem()
	if it == nil || it.kind != itemWindow || it.window.Index != 1 {
		t.Fatalf("cursor not on window 1; got %+v", it)
	}
	if (m.attachTarget != previewKey{}) {
		t.Errorf("jump must not set attachTarget, got %+v", m.attachTarget)
	}
}

func TestJumpModeEscCancels(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "eval"}}})
	start := m.cursor

	m = drive(m, runeKey('s'))
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList", m.mode)
	}
	if m.cursor != start {
		t.Errorf("cursor moved on esc: %d -> %d", start, m.cursor)
	}
}

func TestJumpModeNonLabelCancels(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "eval"}}})

	m = drive(m, runeKey('s'))
	m = drive(m, runeKey('q')) // 'q' is in the alphabet but unassigned (no window rows), so cancels

	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList after non-label key", m.mode)
	}
}

func TestRenderHelp_JumpModeHint(t *testing.T) {
	help := renderHelp(modeJump, "last")
	if !strings.Contains(help, "jump") {
		t.Errorf("jump-mode help should mention jump: %q", help)
	}
	if !strings.Contains(help, "cancel") {
		t.Errorf("jump-mode help should mention cancel: %q", help)
	}
}

func TestRenderHelp_ListModeShowsJumpKey(t *testing.T) {
	help := renderHelp(modeList, "")
	if !strings.Contains(help, "jump") {
		t.Errorf("list-mode help should advertise jump: %q", help)
	}
}

func TestJumpAlphabetExcludesReservedKey(t *testing.T) {
	if len(jumpAlphabet) != 25 {
		t.Fatalf("jumpAlphabet len = %d, want 25 (a-z minus the reserved key)", len(jumpAlphabet))
	}
	seen := map[rune]bool{}
	for _, r := range jumpAlphabet {
		if r < 'a' || r > 'z' {
			t.Errorf("non a-z rune %q in jumpAlphabet", r)
		}
		if seen[r] {
			t.Errorf("duplicate rune %q in jumpAlphabet", r)
		}
		seen[r] = true
	}
	if seen[jumpReserved] {
		t.Errorf("jumpAlphabet must not contain the reserved key %q", jumpReserved)
	}
	if len(seen) != 25 {
		t.Errorf("distinct letters = %d, want 25", len(seen))
	}
}

func TestAssignLabels_ReservesSForLastTarget(t *testing.T) {
	sessions := []tmux.Session{{Name: "mux"}, {Name: "dotfiles"}}
	windows := []tmux.Window{{Index: 0}, {Index: 1}}
	items := []listItem{
		{kind: itemSession, session: &sessions[0]},
		{kind: itemWindow, session: &sessions[0], window: &windows[0]},
		{kind: itemWindow, session: &sessions[0], window: &windows[1]},
		{kind: itemSession, session: &sessions[1]},
		{kind: itemWindow, session: &sessions[1], window: &windows[0]},
	}

	got := assignLabels(items, rowRef{session: "dotfiles", window: 0}, rowRef{})
	// The target takes "s" without consuming a pool letter, so the other
	// window rows keep the labels they would have had anyway.
	want := []string{"", "a", "d", "", "s"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("labels[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// The reserved row does not consume a pool letter regardless of where it sits
// in the list. TestAssignLabels_ReservesSForLastTarget places the target last;
// this places it first, so the pool assignment for the rows after it must be
// identical to what they'd get with no target at all.
func TestAssignLabels_ReservedRowOrderIndependent(t *testing.T) {
	sessions := []tmux.Session{{Name: "dotfiles"}, {Name: "mux"}}
	windows := []tmux.Window{{Index: 0}, {Index: 1}}
	items := []listItem{
		{kind: itemSession, session: &sessions[0]},
		{kind: itemWindow, session: &sessions[0], window: &windows[0]}, // target
		{kind: itemWindow, session: &sessions[0], window: &windows[1]},
		{kind: itemSession, session: &sessions[1]},
		{kind: itemWindow, session: &sessions[1], window: &windows[0]},
	}

	got := assignLabels(items, rowRef{session: "dotfiles", window: 0}, rowRef{})
	want := []string{"", "s", "a", "", "d"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("labels[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAssignLabels_NoRowGetsSWithoutATarget(t *testing.T) {
	sessions := []tmux.Session{{Name: "mux"}}
	windows := []tmux.Window{{Index: 0}, {Index: 1}}
	items := []listItem{
		{kind: itemSession, session: &sessions[0]},
		{kind: itemWindow, session: &sessions[0], window: &windows[0]},
		{kind: itemWindow, session: &sessions[0], window: &windows[1]},
	}

	for i, label := range assignLabels(items, rowRef{}, rowRef{}) {
		if label == string(jumpReserved) {
			t.Errorf("labels[%d] = %q, want no reserved label without a target", i, label)
		}
	}
}

func TestLastTargetMsgStoresTarget(t *testing.T) {
	m := NewModel()
	m = drive(m, lastTargetMsg{session: "dotfiles", window: 2, ok: true})

	if m.lastSession != "dotfiles" {
		t.Errorf("lastSession = %q, want \"dotfiles\"", m.lastSession)
	}
	if m.lastWindow != 2 {
		t.Errorf("lastWindow = %d, want 2", m.lastWindow)
	}
}

func TestLastTargetMsgIgnoredWhenNotOk(t *testing.T) {
	m := NewModel()
	m = drive(m, lastTargetMsg{session: "dotfiles", window: 2, ok: false})

	if m.lastSession != "" {
		t.Errorf("lastSession = %q, want empty when ok=false", m.lastSession)
	}
}

func TestJumpToLastTargetWhenVisible(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "dotfiles"}}})
	m = drive(m, windowsLoadedMsg{sessionName: "dotfiles", windows: []tmux.Window{
		{Index: 1, Name: "nvim"},
		{Index: 2, Name: "claude"},
	}})
	m = drive(m, lastTargetMsg{session: "dotfiles", window: 2, ok: true})

	m = drive(m, runeKey('s')) // enter jump mode
	m = drive(m, runeKey('s')) // reserved: jump to the last target

	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList after jump", m.mode)
	}
	it := m.currentItem()
	if it == nil || it.kind != itemWindow || it.session.Name != "dotfiles" || it.window.Index != 2 {
		t.Fatalf("cursor not on dotfiles window 2; got %+v", it)
	}
	if (m.attachTarget != previewKey{}) {
		t.Errorf("jump must not set attachTarget, got %+v", m.attachTarget)
	}
}

func TestJumpToLastTargetExpandsCollapsedSession(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "dotfiles"}}})
	m = drive(m, lastTargetMsg{session: "dotfiles", window: 2, ok: true})
	m.tree.setSessionExpanded("dotfiles", false) // user collapsed it

	m = drive(m, runeKey('s'))
	m = drive(m, runeKey('s'))

	if !m.tree.isSessionExpanded("dotfiles") {
		t.Error("jump should expand the collapsed target session")
	}
	// dotfiles' windows were never loaded, so the focus must stay pending.
	if m.focusSession != "dotfiles" || m.focusWindow != 2 {
		t.Fatalf("pending focus = (%q, %d), want (\"dotfiles\", 2)", m.focusSession, m.focusWindow)
	}

	m = drive(m, windowsLoadedMsg{sessionName: "dotfiles", windows: []tmux.Window{
		{Index: 1, Name: "nvim"},
		{Index: 2, Name: "claude"},
	}})

	it := m.currentItem()
	if it == nil || it.kind != itemWindow || it.session.Name != "dotfiles" || it.window.Index != 2 {
		t.Fatalf("cursor did not snap to dotfiles window 2 after load; got %+v", it)
	}
}

func TestJumpToLastTargetNoOpWithoutTarget(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "eval"}}})
	start := m.cursor

	m = drive(m, runeKey('s'))
	m = drive(m, runeKey('s'))

	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList", m.mode)
	}
	if m.cursor != start {
		t.Errorf("cursor moved without a target: %d -> %d", start, m.cursor)
	}
}

// A filtered-out target must make `ss` inert: the row is genuinely absent (its
// windows are loaded, so only the filter is hiding it), and the jump must move
// the cursor now or not at all — never arm a focus that fires later when the
// filter is cleared.
func TestJumpToLastTargetHiddenByFilter(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "dotfiles"}}})
	m = drive(m, windowsLoadedMsg{sessionName: "dotfiles", windows: []tmux.Window{
		{Index: 1, Name: "nvim"},
		{Index: 2, Name: "claude"},
	}})
	m = drive(m, lastTargetMsg{session: "dotfiles", window: 2, ok: true})
	m = drive(m, filterAppliedMsg{text: "mux"}) // dotfiles is filtered out
	start := m.cursor

	m = drive(m, runeKey('s'))
	m = drive(m, runeKey('s'))

	if m.cursor != start {
		t.Errorf("cursor moved to a filtered-out target: %d -> %d", start, m.cursor)
	}
	if m.focusSession != "" {
		t.Errorf("focusSession = %q, want empty: ss must not arm a focus for a filtered-out target", m.focusSession)
	}

	m = drive(m, filterAppliedMsg{text: "", cleared: true})
	it := m.currentItem()
	if it != nil && it.session.Name == "dotfiles" {
		t.Errorf("clearing the filter must not move the cursor onto dotfiles; got %+v", it)
	}
}

// Same root cause as the filter-hides-the-row case above, but with the target
// also collapsed: ss must not expand a session the user can't see just because
// it happens to be the jump target.
func TestJumpToLastTargetFilteredAndCollapsedDoesNotExpand(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "dotfiles"}}})
	m.tree.setSessionExpanded("dotfiles", false) // user collapsed it
	m = drive(m, lastTargetMsg{session: "dotfiles", window: 2, ok: true})
	m = drive(m, filterAppliedMsg{text: "mux"}) // dotfiles is filtered out

	m = drive(m, runeKey('s'))
	m = drive(m, runeKey('s'))

	if m.tree.isSessionExpanded("dotfiles") {
		t.Error("ss must not expand a filtered-out collapsed session")
	}
	if m.focusSession != "" {
		t.Errorf("focusSession = %q, want empty for a filtered-out target", m.focusSession)
	}
}

func TestRenderHelp_JumpModeAdvertisesReservedKey(t *testing.T) {
	help := renderHelp(modeJump, "last")
	if !strings.Contains(help, "last") {
		t.Errorf("jump-mode help should mention the reserved last-session key: %q", help)
	}
}

func TestRenderHelp_JumpModeHidesReservedKeyWithoutTarget(t *testing.T) {
	help := renderHelp(modeJump, "")
	if strings.Contains(help, "last") {
		t.Errorf("jump-mode help should not advertise s/last without a target: %q", help)
	}
	if !strings.Contains(help, "jump") || !strings.Contains(help, "cancel") {
		t.Errorf("jump-mode help should still mention jump and cancel: %q", help)
	}
}

// anchorModel builds a Model whose launch window is mux:0 and whose last-session
// target is dotfiles:1, with both sessions' windows loaded.
func anchorModel(t *testing.T) Model {
	t.Helper()
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "dotfiles"}}})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim", Active: true},
		{Index: 1, Name: "zsh"},
	}})
	m = drive(m, windowsLoadedMsg{sessionName: "dotfiles", windows: []tmux.Window{
		{Index: 1, Name: "nvim", Active: true},
		{Index: 2, Name: "claude"},
	}})
	m = drive(m, currentContextMsg{session: "mux", window: 0, ok: true})
	m = drive(m, lastTargetMsg{session: "dotfiles", window: 1, ok: true})
	return m
}

// rowIndex returns the index of the given window row.
func rowIndex(t *testing.T, m Model, session string, window int) int {
	t.Helper()
	for i, it := range m.items {
		if it.kind == itemWindow && it.session.Name == session && it.window.Index == window {
			return i
		}
	}
	t.Fatalf("no window row for %s:%d", session, window)
	return -1
}

func TestCurrentContextRetainsLaunchWindow(t *testing.T) {
	m := anchorModel(t)
	if m.homeSession != "mux" {
		t.Errorf("homeSession = %q, want \"mux\"", m.homeSession)
	}
	if m.homeWindow != 0 {
		t.Errorf("homeWindow = %d, want 0", m.homeWindow)
	}
}

func TestJumpAnchors(t *testing.T) {
	home := rowRef{session: "mux", window: 0}
	last := rowRef{session: "dotfiles", window: 1}

	tests := []struct {
		name       string
		setup      func(m *Model)
		wantTarget rowRef
		wantOther  rowRef
	}{
		{
			name:       "cursor on home points at last",
			setup:      func(m *Model) { m.cursor = rowIndex(t, *m, "mux", 0) },
			wantTarget: last,
			wantOther:  home,
		},
		{
			name:       "cursor on last points home",
			setup:      func(m *Model) { m.cursor = rowIndex(t, *m, "dotfiles", 1) },
			wantTarget: home,
			wantOther:  last,
		},
		{
			name:       "cursor on neither points at last",
			setup:      func(m *Model) { m.cursor = rowIndex(t, *m, "mux", 1) },
			wantTarget: last,
			wantOther:  home,
		},
		{
			name: "no home is one-way",
			setup: func(m *Model) {
				m.homeSession = ""
				m.cursor = rowIndex(t, *m, "dotfiles", 1)
			},
			wantTarget: last,
			wantOther:  rowRef{},
		},
		{
			name:       "no last means no anchors",
			setup:      func(m *Model) { m.lastSession = "" },
			wantTarget: rowRef{},
			wantOther:  rowRef{},
		},
		{
			name: "home equal to last is treated as no home",
			setup: func(m *Model) {
				m.homeSession = "dotfiles"
				m.homeWindow = 1
				m.cursor = rowIndex(t, *m, "dotfiles", 1)
			},
			wantTarget: last,
			wantOther:  rowRef{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := anchorModel(t)
			tc.setup(&m)
			target, other := m.jumpAnchors()
			if target != tc.wantTarget {
				t.Errorf("target = %+v, want %+v", target, tc.wantTarget)
			}
			if other != tc.wantOther {
				t.Errorf("other = %+v, want %+v", other, tc.wantOther)
			}
		})
	}
}

// A pane row under the last window is a different row, so the key still points
// outward rather than home.
func TestJumpAnchors_PaneUnderLastIsNotLast(t *testing.T) {
	m := anchorModel(t)
	m.tree.setWindowExpanded("dotfiles", 1, true)
	m = drive(m, panesLoadedMsg{sessionName: "dotfiles", windowIndex: 1, panes: []tmux.Pane{
		{Index: 0, Command: "zsh", Active: true},
	}})
	m.cursor = rowIndex(t, m, "dotfiles", 1) + 1 // the pane row

	target, _ := m.jumpAnchors()
	if want := (rowRef{session: "dotfiles", window: 1}); target != want {
		t.Errorf("target = %+v, want %+v (pane row must not count as the last row)", target, want)
	}
}

// The pool must not depend on the cursor. Both anchors are reserved at all
// times, so only which of them shows `s` changes as the cursor moves — every
// other row keeps its letter. This is the whole reason both ends are reserved.
func TestAssignLabels_PoolIsStableAcrossCursorPositions(t *testing.T) {
	positions := []struct {
		name    string
		session string
		window  int
	}{
		{"cursor on home", "mux", 0},
		{"cursor on last", "dotfiles", 1},
		{"cursor on neither", "mux", 1},
	}

	var reference []string
	for _, p := range positions {
		m := anchorModel(t)
		m.cursor = rowIndex(t, m, p.session, p.window)
		target, other := m.jumpAnchors()
		labels := assignLabels(m.items, target, other)

		// Collect the labels of every row that is not an anchor.
		var pool []string
		for i, it := range m.items {
			if it.kind != itemWindow || target.matches(it) || other.matches(it) {
				continue
			}
			pool = append(pool, labels[i])
		}
		if reference == nil {
			reference = pool
			continue
		}
		if len(pool) != len(reference) {
			t.Fatalf("%s: pool size %d, want %d", p.name, len(pool), len(reference))
		}
		for i := range pool {
			if pool[i] != reference[i] {
				t.Errorf("%s: pool[%d] = %q, want %q (pool must not move with the cursor)",
					p.name, i, pool[i], reference[i])
			}
		}
	}
}

func TestAssignLabels_TargetGetsSOtherGetsDot(t *testing.T) {
	m := anchorModel(t)
	m.cursor = rowIndex(t, m, "mux", 0) // on home, so the key points at last
	target, other := m.jumpAnchors()
	labels := assignLabels(m.items, target, other)

	if got := labels[rowIndex(t, m, "dotfiles", 1)]; got != string(jumpReserved) {
		t.Errorf("target label = %q, want %q", got, string(jumpReserved))
	}
	if got := labels[rowIndex(t, m, "mux", 0)]; got != string(jumpInactive) {
		t.Errorf("other label = %q, want %q", got, string(jumpInactive))
	}
}

// The toggle flips which end carries `s` when the cursor sits on the last row.
func TestAssignLabels_ToggleFlipsWhichEndCarriesS(t *testing.T) {
	m := anchorModel(t)
	m.cursor = rowIndex(t, m, "dotfiles", 1) // on last, so the key points home
	target, other := m.jumpAnchors()
	labels := assignLabels(m.items, target, other)

	if got := labels[rowIndex(t, m, "mux", 0)]; got != string(jumpReserved) {
		t.Errorf("home label = %q, want %q", got, string(jumpReserved))
	}
	if got := labels[rowIndex(t, m, "dotfiles", 1)]; got != string(jumpInactive) {
		t.Errorf("last label = %q, want %q", got, string(jumpInactive))
	}
}

// An anchor whose row is not in the list labels nothing and consumes no letter.
func TestAssignLabels_AbsentAnchorConsumesNothing(t *testing.T) {
	m := anchorModel(t)
	withAnchors := assignLabels(m.items, rowRef{session: "mux", window: 0}, rowRef{})
	offList := assignLabels(m.items, rowRef{session: "ghost", window: 9}, rowRef{})

	// mux:0 is reserved in the first call and an ordinary pool row in the
	// second, so the pool advances by one more row there.
	if withAnchors[rowIndex(t, m, "mux", 0)] != string(jumpReserved) {
		t.Fatalf("setup: mux:0 should carry the reserved label")
	}
	if offList[rowIndex(t, m, "mux", 0)] != string(jumpAlphabet[0]) {
		t.Errorf("off-list anchor should not reserve anything; mux:0 = %q, want %q",
			offList[rowIndex(t, m, "mux", 0)], string(jumpAlphabet[0]))
	}
}

// rebuildItems assigns labels AFTER applyPendingFocus moves the cursor. With
// the order reversed the labels describe the pre-move cursor.
func TestRebuildItemsLabelsFollowThePostFocusCursor(t *testing.T) {
	m := anchorModel(t)
	m.cursor = rowIndex(t, m, "mux", 0)

	// Arm a pending focus for the last row and rebuild, exactly as `ss` does.
	m.focusSession = "dotfiles"
	m.focusWindow = 1
	m.rebuildItems()

	if m.cursor != rowIndex(t, m, "dotfiles", 1) {
		t.Fatalf("setup: cursor did not land on the last row")
	}
	// Cursor now sits on last, so the key must point home and home must carry s.
	if got := m.labels[rowIndex(t, m, "mux", 0)]; got != string(jumpReserved) {
		t.Errorf("home label = %q, want %q — labels were assigned before the cursor settled",
			got, string(jumpReserved))
	}
}

func TestReservedKeyTogglesBackHome(t *testing.T) {
	m := anchorModel(t)
	m.cursor = rowIndex(t, m, "mux", 0)

	m = drive(m, runeKey('s'))
	m = drive(m, runeKey('s'))
	if want := rowIndex(t, m, "dotfiles", 1); m.cursor != want {
		t.Fatalf("first ss: cursor = %d, want %d (last row)", m.cursor, want)
	}

	m = drive(m, runeKey('s'))
	m = drive(m, runeKey('s'))
	if want := rowIndex(t, m, "mux", 0); m.cursor != want {
		t.Fatalf("second ss: cursor = %d, want %d (home row)", m.cursor, want)
	}
	if (m.attachTarget != previewKey{}) {
		t.Errorf("toggle must not attach, got %+v", m.attachTarget)
	}
}

// From a row that is neither anchor, the key goes outward, not home.
func TestReservedKeyFromThirdRowGoesToLast(t *testing.T) {
	m := anchorModel(t)
	m.cursor = rowIndex(t, m, "mux", 1)

	m = drive(m, runeKey('s'))
	m = drive(m, runeKey('s'))

	if want := rowIndex(t, m, "dotfiles", 1); m.cursor != want {
		t.Errorf("cursor = %d, want %d (last row)", m.cursor, want)
	}
}

func TestRenderHelp_ReservedHint(t *testing.T) {
	if help := renderHelp(modeJump, "last"); !strings.Contains(help, "last") {
		t.Errorf("help should show the outbound hint: %q", help)
	}
	if help := renderHelp(modeJump, "back"); !strings.Contains(help, "back") {
		t.Errorf("help should show the return hint: %q", help)
	}
	help := renderHelp(modeJump, "")
	if strings.Contains(help, "last") || strings.Contains(help, "back") {
		t.Errorf("help should omit the segment with no target: %q", help)
	}
	if !strings.Contains(help, "jump") || !strings.Contains(help, "cancel") {
		t.Errorf("help lost its other segments: %q", help)
	}
}

func TestReservedHintFollowsTheAnchors(t *testing.T) {
	m := anchorModel(t)

	m.cursor = rowIndex(t, m, "mux", 0)
	if got := m.reservedHint(); got != "last" {
		t.Errorf("hint on home = %q, want \"last\"", got)
	}

	m.cursor = rowIndex(t, m, "dotfiles", 1)
	if got := m.reservedHint(); got != "back" {
		t.Errorf("hint on last = %q, want \"back\"", got)
	}

	m.lastSession = ""
	if got := m.reservedHint(); got != "" {
		t.Errorf("hint with no target = %q, want empty", got)
	}
}

// Regression: every cursor-movement key must refresh m.labels immediately, not
// just on the next rebuild. ss's reserved-key branch already calls
// rebuildItems, so it legitimately lands the cursor on the last row with
// correct labels (home shows the reserved key, last shows the dot). An
// ordinary movement key off that row must bring the labels along with it —
// jumpAnchors swaps which end is the target the moment the cursor leaves last,
// and the label the list shows must agree with where `s` would actually go.
func TestMovementRefreshesLabels(t *testing.T) {
	m := anchorModel(t)
	m = drive(m, runeKey('s'))
	m = drive(m, runeKey('s'))
	if want := rowIndex(t, m, "dotfiles", 1); m.cursor != want {
		t.Fatalf("setup: cursor = %d, want %d (last row)", m.cursor, want)
	}

	m = drive(m, tea.KeyMsg{Type: tea.KeyUp})

	target, other := m.jumpAnchors()
	if target.present() {
		idx := rowIndex(t, m, target.session, target.window)
		if got := m.labels[idx]; got != string(jumpReserved) {
			t.Errorf("target row label = %q, want %q (m.labels did not follow the cursor)", got, string(jumpReserved))
		}
	}
	if other.present() {
		idx := rowIndex(t, m, other.session, other.window)
		if got := m.labels[idx]; got != string(jumpInactive) {
			t.Errorf("other row label = %q, want %q (m.labels did not follow the cursor)", got, string(jumpInactive))
		}
	}
}

// The dot is not a key you can press, so jump mode must not brighten it.
// The profile must be forced: lipgloss strips color when stdout is not a TTY,
// which would make both comparisons below trivially equal and the test vacuous.
func TestStyleRowKeepsTheInactiveDotMuted(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	base := lipgloss.NewStyle()
	dotResting := styleRow(" row", base, string(jumpInactive), false)
	dotActive := styleRow(" row", base, string(jumpInactive), true)
	if dotResting != dotActive {
		t.Errorf("the inactive dot changed with jump mode:\nrest:   %q\nactive: %q", dotResting, dotActive)
	}

	sResting := styleRow(" row", base, string(jumpReserved), false)
	sActive := styleRow(" row", base, string(jumpReserved), true)
	if sResting == sActive {
		t.Errorf("the reserved label should still brighten in jump mode: %q", sActive)
	}
}
