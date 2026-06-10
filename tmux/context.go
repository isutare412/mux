package tmux

import (
	"os"
	"strconv"
	"strings"
)

// currentContextFormat is the display-message format for the launching pane's
// session name and window index.
const currentContextFormat = "#{session_name}|#{window_index}"

// CurrentContext returns the session name and window index mux was launched
// from. ok is false when mux is not running inside tmux ($TMUX unset) or
// detection fails, in which case the caller should fall back to its default
// cursor position.
//
// A direct launch sets $TMUX_PANE to the launching pane, which we target
// explicitly. A `display-popup` launch leaves $TMUX_PANE empty, so we fall back
// to `display-message` without -t, which resolves the client's current pane —
// the window the popup was opened from.
func CurrentContext() (session string, window int, ok bool) {
	if os.Getenv("TMUX") == "" {
		return "", 0, false
	}

	args := []string{"display-message", "-p", currentContextFormat}
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		args = []string{"display-message", "-t", pane, "-p", currentContextFormat}
	}

	out, err := runner.Output("tmux", args...)
	if err != nil {
		return "", 0, false
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	if len(parts) < 2 {
		return "", 0, false
	}
	if parts[0] == "" {
		return "", 0, false
	}

	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}
	return parts[0], idx, true
}
