package ui

// sessionStops returns the item indices J/K move between: one per session. A
// session's stop is its starred window row — the window tmux reports as active
// — when that row is visible; otherwise the session's own row, which covers a
// collapsed session, one whose windows have not loaded yet, and the defensive
// case of no window carrying the active flag.
//
// Pane rows are never stops. formatPaneRow marks the active pane with the same
// star, but only windows are navigation targets.
//
// The result is ascending: each stop sits at or after its session row and
// before the next one.
func sessionStops(items []listItem) []int {
	var stops []int
	for i := 0; i < len(items); i++ {
		if items[i].kind != itemSession {
			continue
		}
		stop := i
		// Scan this session's subtree, which ends at the next session row.
		for j := i + 1; j < len(items) && items[j].kind != itemSession; j++ {
			if items[j].kind == itemWindow && items[j].window.Active {
				stop = j
				break
			}
		}
		stops = append(stops, stop)
	}
	return stops
}

// nextSessionStop returns the first session stop after from when dir is +1, or
// the last one before from when dir is -1. ok is false at the ends: the cursor
// stays put rather than wrapping, matching j/k. from need not be a stop itself
// — the comparison is positional.
func nextSessionStop(items []listItem, from, dir int) (int, bool) {
	stops := sessionStops(items)
	if dir > 0 {
		for _, s := range stops {
			if s > from {
				return s, true
			}
		}
		return 0, false
	}
	for i := len(stops) - 1; i >= 0; i-- {
		if stops[i] < from {
			return stops[i], true
		}
	}
	return 0, false
}
