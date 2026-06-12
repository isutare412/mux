package ui

// jumpAlphabet is the ordered set of single-key labels used by jump mode.
// Home-row keys come first for ergonomics. It covers the whole alphabet (26
// letters): the original ergonomic ordering forms the prefix, with the
// remaining letters appended alphabetically. Because every letter is now a
// label, a command key (j/k/etc.) pressed in jump mode jumps to its row rather
// than cancelling — use esc to cancel.
const jumpAlphabet = "asdfwecvbtyuiopmzghjklnqrx"

// assignLabels walks the flattened item list top-to-bottom and assigns the next
// letter from jumpAlphabet to each WINDOW row. Session rows, pane rows, and any
// rows beyond the alphabet receive "" (no label; still reachable via j/k). The
// result is a slice parallel to items.
func assignLabels(items []listItem) []string {
	labels := make([]string, len(items))
	next := 0
	for i, it := range items {
		if it.kind != itemWindow {
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
