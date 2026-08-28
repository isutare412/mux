package ui

// jumpReserved is the label permanently held back from the pool below. It is
// handed to whichever end of the home/last toggle is currently the target —
// the launch window when the cursor sits on the last session, the last
// session otherwise — so `ss` always jumps to that row and, from there, back.
const jumpReserved = 's'

// jumpAlphabet is the ordered set of single-key labels used by jump mode.
// Home-row keys come first for ergonomics. It covers every letter except
// jumpReserved (25 of them): the original ergonomic ordering forms the prefix,
// with the remaining letters appended alphabetically. Because nearly every
// letter is a label, a command key (j/k/etc.) pressed in jump mode jumps to its
// row rather than cancelling — use esc to cancel.
const jumpAlphabet = "adfwecvbtyuiopmzghjklnqrx"

// jumpInactive marks the other end of the reserved key's toggle — the anchor
// `s` is not currently pointing at. It is never pressable, so it renders muted
// in every mode. U+00B7 is one cell wide, matching the single-column label slot.
const jumpInactive = '·'

// jumpInactiveLabel is jumpInactive pre-converted to a string, since assignLabels
// and applyJumpLabel each need the string form once per row per frame.
var jumpInactiveLabel = string(jumpInactive)

// assignLabels walks the flattened item list top-to-bottom and assigns a letter
// to each WINDOW row. The target row gets jumpReserved and the other end of the
// toggle gets jumpInactive; neither consumes a letter from jumpAlphabet.
//
// Reserving BOTH anchors, not just whichever one is currently the target, is
// what keeps the pool stable: the set of skipped rows never changes as the
// cursor moves, so every other row keeps its letter. An absent anchor, or one
// whose row is not in items, simply matches nothing.
//
// Session rows, pane rows, and any rows beyond the alphabet receive "" (no
// label; still reachable via j/k). The result is a slice parallel to items.
func assignLabels(items []listItem, target, other rowRef) []string {
	labels := make([]string, len(items))
	next := 0
	for i, it := range items {
		if it.kind != itemWindow {
			continue
		}
		if target.matches(it) {
			labels[i] = string(jumpReserved)
			continue
		}
		if other.matches(it) {
			labels[i] = jumpInactiveLabel
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
