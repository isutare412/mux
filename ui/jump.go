package ui

// jumpReserved is the label permanently held back from the pool below. It is
// handed to the last session's active window row instead, so `ss` always lands
// on the session the user came from no matter how the tree is shaped.
const jumpReserved = 's'

// jumpAlphabet is the ordered set of single-key labels used by jump mode.
// Home-row keys come first for ergonomics. It covers every letter except
// jumpReserved (25 of them): the original ergonomic ordering forms the prefix,
// with the remaining letters appended alphabetically. Because nearly every
// letter is a label, a command key (j/k/etc.) pressed in jump mode jumps to its
// row rather than cancelling — use esc to cancel.
const jumpAlphabet = "adfwecvbtyuiopmzghjklnqrx"

// assignLabels walks the flattened item list top-to-bottom and assigns a letter
// to each WINDOW row. The row matching (lastSession, lastWindow) gets
// jumpReserved without consuming a letter from jumpAlphabet, so every other row
// keeps the label it would have had regardless of where the target sits — or
// whether there is one. Session rows, pane rows, and any rows beyond the
// alphabet receive "" (no label; still reachable via j/k). The result is a slice
// parallel to items.
func assignLabels(items []listItem, lastSession string, lastWindow int) []string {
	labels := make([]string, len(items))
	next := 0
	for i, it := range items {
		if it.kind != itemWindow {
			continue
		}
		if lastSession != "" && it.session.Name == lastSession && it.window.Index == lastWindow {
			labels[i] = string(jumpReserved)
			continue
		}
		if next >= len(jumpAlphabet) {
			continue // overflow: leave label empty
		}
		labels[i] = string(jumpAlphabet[next])
		next++
	}
	return labels
}

// rowRef names a window row by session name and window index. The zero value
// means absent.
type rowRef struct {
	session string
	window  int
}

// present reports whether this ref names a row at all.
func (r rowRef) present() bool { return r.session != "" }

// matches reports whether it is the window row r names. Only window rows match:
// a pane row beneath that window is a different row.
func (r rowRef) matches(it listItem) bool {
	return r.present() && it.kind == itemWindow &&
		it.session.Name == r.session && it.window.Index == r.window
}

// jumpAnchors returns the row the reserved key jumps to, and the other end of
// the toggle. Standing on the last-session row points the key home; anywhere
// else points it at the last session. Either may be absent.
//
// Both the label assignment and the key handler resolve through this one
// function, so what the list advertises and where the key goes cannot drift
// apart.
func (m *Model) jumpAnchors() (target, other rowRef) {
	last := rowRef{session: m.lastSession, window: m.lastWindow}
	home := rowRef{session: m.homeSession, window: m.homeWindow}

	if !last.present() {
		return rowRef{}, rowRef{}
	}
	// No launch context (mux started outside tmux), or the two ends coincide:
	// one-way, as before. LastTarget excludes the current session, so the
	// equality case is defensive.
	if !home.present() || home == last {
		return last, rowRef{}
	}
	if it := m.currentItem(); it != nil && last.matches(*it) {
		return home, last
	}
	return last, home
}
