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
