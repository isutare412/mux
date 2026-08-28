package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// clickZone names the part of a list row a mouse coordinate landed on.
type clickZone int

const (
	zoneNone clickZone = iota // outside the list panel, or past the last row
	zoneRow
	zoneChevron
)

// rowAt maps a screen coordinate to a list row and the zone within it.
// Returns (-1, zoneNone) when the coordinate is not over a row.
//
// The geometry comes from panelGeometry and the scroll position from
// listScrollOffset — the same two the renderer draws through, so the row this
// returns is the row under the pointer even after the list has scrolled or the
// filter bar has pushed everything down a line.
func (m Model) rowAt(x, y int) (int, clickZone) {
	if !m.overList(x, y) {
		return -1, zoneNone
	}

	g := m.panelGeometry()
	idx := listScrollOffset(m.cursor, g.height-2) + (y - g.top - 1)
	if idx >= len(m.items) {
		return -1, zoneNone
	}

	if col := chevronColumn(m.items[idx]); col >= 0 && x-1 <= col {
		return idx, zoneChevron
	}
	return idx, zoneRow
}

// overList reports whether a screen coordinate falls inside the list panel's
// interior — borders excluded, blank space below the last row included. The
// wheel scrolls from anywhere in there, not just from an occupied row.
func (m Model) overList(x, y int) bool {
	g := m.panelGeometry()
	first := g.top + 1
	return y >= first && y < first+g.height-2 && x >= 1 && x < g.listWidth-1
}

// chevronColumn returns the interior column holding the row's expand/collapse
// chevron, or -1 for rows that have none. The indent in front of the chevron
// counts as part of it, so the click target is not a single cell.
func chevronColumn(it listItem) int {
	switch it.kind {
	case itemSession:
		return 0
	case itemWindow:
		return indentWindow
	}
	return -1
}

// updateMouse routes a mouse event.
//
// A press inside the list moves the cursor there, and a second press on the
// same row within doubleClickInterval attaches — the same destination enter
// would pick. A press on the row's chevron toggles that row's expansion
// instead, so it never attaches. The wheel walks the cursor one row at a time
// while the pointer is over the list. Everything else is ignored: motion and
// release events (a physical click sends both, and only the press may count),
// coordinates over the preview, and every event that arrives while a modal
// overlay owns the screen.
func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeCreate, modeRename, modeConfirmKill:
		return m, nil
	case modeJump:
		// Reaching for the mouse abandons the jump.
		m.mode = modeList
	}

	if msg.Action != tea.MouseActionPress {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.overList(msg.X, msg.Y) && m.cursor > 0 {
			return m, m.moveCursor(m.cursor - 1)
		}
	case tea.MouseButtonWheelDown:
		if m.overList(msg.X, msg.Y) && m.cursor < len(m.items)-1 {
			return m, m.moveCursor(m.cursor + 1)
		}
	case tea.MouseButtonLeft:
		return m.pressRow(msg.X, msg.Y)
	}
	return m, nil
}

// pressRow handles a left press at a screen coordinate.
func (m Model) pressRow(x, y int) (tea.Model, tea.Cmd) {
	idx, zone := m.rowAt(x, y)
	if zone == zoneNone {
		return m, nil
	}

	repeat := idx == m.lastClickRow && time.Since(m.lastClickAt) < doubleClickInterval
	m.lastClickRow = idx
	m.lastClickAt = time.Now()

	cmd := m.moveCursor(idx)

	if zone == zoneChevron {
		var toggle tea.Cmd
		var next tea.Model
		if m.rowExpanded(idx) {
			next, toggle = m.collapseCurrent()
		} else {
			next, toggle = m.expandCurrent()
		}
		return next, tea.Batch(cmd, toggle)
	}

	if repeat {
		if it := m.currentItem(); it != nil {
			m.attachTarget = previewKeyForItem(*it)
			return m, tea.Quit
		}
	}
	return m, cmd
}

// rowExpanded reports whether the row at idx is currently expanded. Rows that
// cannot expand at all report false, so a chevron press on one tries to expand
// it — which is a no-op — rather than collapsing its parent the way the
// keyboard's ⇧tab does.
func (m Model) rowExpanded(idx int) bool {
	it := m.items[idx]
	switch it.kind {
	case itemSession:
		return m.tree.isSessionExpanded(it.session.Name)
	case itemWindow:
		return m.tree.isWindowExpanded(it.session.Name, it.window.Index)
	}
	return false
}
