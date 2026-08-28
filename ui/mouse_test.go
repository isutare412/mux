package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lunemis/mux/tmux"
)

// listModel returns a sized model holding one session with two windows.
// items: [0] session mux, [1] window 0 nvim, [2] window 1 claude, [3] session dotfiles
func listModel(t *testing.T, width, height int, opts ...Option) Model {
	t.Helper()
	m := NewModel(opts...)
	m = drive(m, tea.WindowSizeMsg{Width: width, Height: height})
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "dotfiles"}}})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim"},
		{Index: 1, Name: "claude"},
	}})
	m.cursor = 0
	return m
}

func TestRowAtMapsScreenRowToItem(t *testing.T) {
	m := listModel(t, 100, 30)
	if len(m.items) != 4 {
		t.Fatalf("items = %d, want 4", len(m.items))
	}

	// Title occupies row 0, the panel's top border row 1, so the first item
	// row lands on screen row 2.
	for _, tc := range []struct{ y, want int }{{2, 0}, {3, 1}, {4, 2}, {5, 3}} {
		got, zone := m.rowAt(10, tc.y)
		if zone == zoneNone || got != tc.want {
			t.Errorf("rowAt(10, %d) = (%d, %v), want (%d, a row)", tc.y, got, zone, tc.want)
		}
	}
}

func TestRowAtRejectsOutsideTheList(t *testing.T) {
	m := listModel(t, 100, 30)

	cases := []struct {
		name string
		x, y int
	}{
		{"title row", 10, 0},
		{"top border", 10, 1},
		{"below the last item", 10, 6},
		{"left border", 0, 2},
		{"preview panel", 60, 2},
		{"right border of the list panel", 39, 2},
	}
	for _, tc := range cases {
		if idx, zone := m.rowAt(tc.x, tc.y); zone != zoneNone {
			t.Errorf("%s: rowAt(%d, %d) = (%d, %v), want zoneNone", tc.name, tc.x, tc.y, idx, zone)
		}
	}
}

func TestRowAtShiftsDownForTheFilterBar(t *testing.T) {
	m := listModel(t, 100, 30)
	m.filterText = "mu"
	m.applyFilter()

	// The filter reminder takes a line between the title and the panel, so
	// every row moves down one.
	if idx, zone := m.rowAt(10, 3); zone == zoneNone || idx != 0 {
		t.Errorf("rowAt(10, 3) = (%d, %v), want (0, a row)", idx, zone)
	}
	if idx, zone := m.rowAt(10, 2); zone != zoneNone {
		t.Errorf("rowAt(10, 2) = (%d, %v), want zoneNone (that row is the top border now)", idx, zone)
	}
}

func TestRowAtAccountsForScrollOffset(t *testing.T) {
	m := NewModel()
	m = drive(m, tea.WindowSizeMsg{Width: 100, Height: 10})
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}}})
	windows := make([]tmux.Window, 7)
	for i := range windows {
		windows[i] = tmux.Window{Index: i, Name: "w"}
	}
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: windows})
	if len(m.items) != 8 {
		t.Fatalf("items = %d, want 8", len(m.items))
	}
	// height 10 - chrome 3 = panel 7 => 5 visible rows. A cursor at 6 scrolls
	// the viewport down by 2.
	m.cursor = 6

	if idx, zone := m.rowAt(10, 2); zone == zoneNone || idx != 2 {
		t.Errorf("first visible row = (%d, %v), want (2, a row)", idx, zone)
	}
	if idx, zone := m.rowAt(10, 6); zone == zoneNone || idx != 6 {
		t.Errorf("last visible row = (%d, %v), want (6, a row)", idx, zone)
	}
}

func TestRowAtDetectsTheChevronZone(t *testing.T) {
	m := listModel(t, 100, 30)

	cases := []struct {
		name string
		x, y int
		want clickZone
	}{
		{"session chevron", 1, 2, zoneChevron},
		{"session name", 5, 2, zoneRow},
		{"window indent", 1, 3, zoneChevron},
		{"window chevron", 3, 3, zoneChevron},
		{"window name", 8, 3, zoneRow},
	}
	for _, tc := range cases {
		if _, zone := m.rowAt(tc.x, tc.y); zone != tc.want {
			t.Errorf("%s: rowAt(%d, %d) zone = %v, want %v", tc.name, tc.x, tc.y, zone, tc.want)
		}
	}
}

func TestRowAtTreatsPaneRowsAsRowsOnly(t *testing.T) {
	m := listModel(t, 100, 30)
	m.tree.setWindowExpanded("mux", 0, true)
	m = drive(m, panesLoadedMsg{sessionName: "mux", windowIndex: 0, panes: []tmux.Pane{
		{Index: 0, Command: "zsh"},
	}})
	// items: [0] mux, [1] window 0, [2] pane 0, [3] window 1, [4] dotfiles
	if it := m.items[2]; it.kind != itemPane {
		t.Fatalf("items[2] kind = %v, want itemPane", it.kind)
	}

	// A pane row has no chevron, so its leading cells are just part of the row.
	if idx, zone := m.rowAt(1, 4); zone != zoneRow || idx != 2 {
		t.Errorf("rowAt(1, 4) = (%d, %v), want (2, zoneRow)", idx, zone)
	}
}

// The hit test computes the panel geometry independently of View. If the two
// ever disagree, clicks land on the wrong row, so pin them together: find the
// screen line the renderer actually drew a known session on and feed it back.
func TestRowAtAgreesWithTheRenderedView(t *testing.T) {
	m := listModel(t, 100, 30)
	m.cursor = 3 // dotfiles

	lines := strings.Split(m.View(), "\n")
	y := -1
	for i, line := range lines {
		// View joins the two panels on one line, so clip to the list panel's
		// width — the preview header carries the session name too.
		if strings.Contains(ansi.Truncate(line, m.panelGeometry().listWidth, ""), "dotfiles") {
			y = i
			break
		}
	}
	if y < 0 {
		t.Fatalf("session 'dotfiles' not found in the rendered view")
	}

	idx, zone := m.rowAt(10, y)
	if zone == zoneNone || idx != 3 {
		t.Errorf("rowAt(10, %d) = (%d, %v), want (3, a row) — hit test and View disagree", y, idx, zone)
	}
}

// click builds the press event a left button emits at (x, y).
func click(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func wheel(x, y int, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: button, Action: tea.MouseActionPress}
}

func TestClickMovesTheCursor(t *testing.T) {
	m := listModel(t, 100, 30)

	m = drive(m, click(10, 5)) // row 3: session dotfiles

	if m.cursor != 3 {
		t.Errorf("cursor = %d, want 3", m.cursor)
	}
	if (m.attachTarget != previewKey{}) {
		t.Errorf("a single click must not attach, got %+v", m.attachTarget)
	}
}

func TestClickOutsideTheListLeavesTheCursorAlone(t *testing.T) {
	m := listModel(t, 100, 30)
	m.cursor = 1

	m = drive(m, click(60, 5)) // preview panel

	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (unchanged)", m.cursor)
	}
}

func TestDoubleClickAttaches(t *testing.T) {
	m := listModel(t, 100, 30)

	m = drive(m, click(10, 4)) // row 2: window 1 of mux
	m = drive(m, click(10, 4))

	want := previewKey{session: "mux", window: 1, pane: -1}
	if m.attachTarget != want {
		t.Errorf("attachTarget = %+v, want %+v", m.attachTarget, want)
	}
}

func TestDoubleClickQuits(t *testing.T) {
	m := listModel(t, 100, 30)

	m = drive(m, click(10, 4))
	next, cmd := m.Update(click(10, 4))
	m = next.(Model)

	if cmd == nil {
		t.Fatal("double click returned no command; want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("command produced %T, want tea.QuitMsg", cmd())
	}
}

func TestSlowSecondClickOnlyMoves(t *testing.T) {
	m := listModel(t, 100, 30)

	m = drive(m, click(10, 4))
	m.lastClickAt = m.lastClickAt.Add(-2 * doubleClickInterval)
	m = drive(m, click(10, 4))

	if (m.attachTarget != previewKey{}) {
		t.Errorf("clicks too far apart must not attach, got %+v", m.attachTarget)
	}
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor)
	}
}

func TestQuickClicksOnDifferentRowsDoNotAttach(t *testing.T) {
	m := listModel(t, 100, 30)

	m = drive(m, click(10, 3))
	m = drive(m, click(10, 4))

	if (m.attachTarget != previewKey{}) {
		t.Errorf("two rows, not a double click; got %+v", m.attachTarget)
	}
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor)
	}
}

// A physical click emits a press and then a release. Only the press may count,
// or every click would look like a double click and attach immediately.
func TestReleaseIsNotASecondClick(t *testing.T) {
	m := listModel(t, 100, 30)

	m = drive(m, click(10, 4))
	m = drive(m, tea.MouseMsg{X: 10, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	if (m.attachTarget != previewKey{}) {
		t.Errorf("release must not attach, got %+v", m.attachTarget)
	}
}

func TestChevronClickCollapsesAndExpands(t *testing.T) {
	m := listModel(t, 100, 30)
	if !m.tree.isSessionExpanded("mux") {
		t.Fatal("session mux should start expanded")
	}

	m = drive(m, click(1, 2)) // chevron of the mux session row
	if m.tree.isSessionExpanded("mux") {
		t.Error("chevron click did not collapse the session")
	}
	if (m.attachTarget != previewKey{}) {
		t.Errorf("chevron click must not attach, got %+v", m.attachTarget)
	}

	m = drive(m, click(1, 2))
	if !m.tree.isSessionExpanded("mux") {
		t.Error("second chevron click did not expand the session again")
	}
}

func TestChevronClickMovesTheCursorToThatRow(t *testing.T) {
	m := listModel(t, 100, 30)
	m.cursor = 0

	m = drive(m, click(3, 3)) // chevron of window 0

	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (the row whose chevron was clicked)", m.cursor)
	}
}

func TestWheelMovesTheCursorOneRow(t *testing.T) {
	m := listModel(t, 100, 30)
	m.cursor = 2

	m = drive(m, wheel(10, 4, tea.MouseButtonWheelDown))
	if m.cursor != 3 {
		t.Errorf("cursor after wheel down = %d, want 3", m.cursor)
	}

	m = drive(m, wheel(10, 4, tea.MouseButtonWheelUp))
	if m.cursor != 2 {
		t.Errorf("cursor after wheel up = %d, want 2", m.cursor)
	}
}

func TestWheelStopsAtTheEnds(t *testing.T) {
	m := listModel(t, 100, 30)

	m.cursor = 0
	m = drive(m, wheel(10, 4, tea.MouseButtonWheelUp))
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (already at the top)", m.cursor)
	}

	m.cursor = len(m.items) - 1
	m = drive(m, wheel(10, 4, tea.MouseButtonWheelDown))
	if m.cursor != len(m.items)-1 {
		t.Errorf("cursor = %d, want %d (already at the bottom)", m.cursor, len(m.items)-1)
	}
}

func TestWheelOverThePreviewIsIgnored(t *testing.T) {
	m := listModel(t, 100, 30)
	m.cursor = 2

	m = drive(m, wheel(60, 4, tea.MouseButtonWheelDown))

	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (wheel outside the list does nothing)", m.cursor)
	}
}

func TestMouseIgnoredWhileAnOverlayIsOpen(t *testing.T) {
	m := listModel(t, 100, 30)
	m = drive(m, runeKey('n')) // new-session overlay
	if m.mode != modeCreate {
		t.Fatalf("mode = %v, want modeCreate", m.mode)
	}

	m = drive(m, click(10, 5))

	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (the overlay owns the screen)", m.cursor)
	}
	if m.mode != modeCreate {
		t.Errorf("mode = %v, want modeCreate", m.mode)
	}
}

func TestClickLeavesJumpMode(t *testing.T) {
	m := listModel(t, 100, 30)
	m = drive(m, runeKey('s'))
	if m.mode != modeJump {
		t.Fatalf("mode = %v, want modeJump", m.mode)
	}

	m = drive(m, click(10, 5))

	if m.mode != modeList {
		t.Errorf("mode = %v, want modeList after a click", m.mode)
	}
	if m.cursor != 3 {
		t.Errorf("cursor = %d, want 3", m.cursor)
	}
}

func TestInvertedWheelWalksTheCursorTheOtherWay(t *testing.T) {
	m := listModel(t, 100, 30, WithInvertedScroll())
	m.cursor = 2

	m = drive(m, wheel(10, 4, tea.MouseButtonWheelDown))
	if m.cursor != 1 {
		t.Errorf("cursor after wheel down = %d, want 1", m.cursor)
	}

	m = drive(m, wheel(10, 4, tea.MouseButtonWheelUp))
	if m.cursor != 2 {
		t.Errorf("cursor after wheel up = %d, want 2", m.cursor)
	}
}

func TestInvertedWheelStopsAtTheEnds(t *testing.T) {
	m := listModel(t, 100, 30, WithInvertedScroll())
	m.cursor = 0

	m = drive(m, wheel(10, 4, tea.MouseButtonWheelDown))

	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (already at the top)", m.cursor)
	}
}
