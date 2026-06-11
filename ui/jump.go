package ui

// jumpAlphabet is the ordered set of single-key labels used by jump mode.
// Home-row keys come first for ergonomics. It deliberately EXCLUDES the
// command keys (j k l h g n x r q) so that, while jump mode is active, a
// habitual command keypress is not a label and simply cancels jump mode
// instead of teleporting the cursor to an unexpected row.
const jumpAlphabet = "asdfwecvbtyuiopmz"

// assignLabels walks the flattened item list top-to-bottom and assigns the next
// letter from jumpAlphabet to each session or window row. Pane rows, and any
// rows beyond the alphabet, receive "" (no label; still reachable via j/k). The
// result is a slice parallel to items.
func assignLabels(items []listItem) []string {
	labels := make([]string, len(items))
	next := 0
	for i, it := range items {
		if it.kind == itemPane {
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
