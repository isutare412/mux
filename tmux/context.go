package tmux

import (
	"os"
	"strconv"
	"strings"
)

// currentContextFormat is the display-message format for the launching pane's
// session name and window index.
const currentContextFormat = "#{session_name}|#{window_index}"

// CurrentContext returns the session name and window index of the pane mux was
// launched from. ok is false when mux is not running inside tmux ($TMUX_PANE
// unset) or detection fails, in which case the caller should fall back to its
// default cursor position.
func CurrentContext() (session string, window int, ok bool) {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return "", 0, false
	}

	out, err := runner.Output("tmux", "display-message", "-t", pane, "-p", currentContextFormat)
	if err != nil {
		return "", 0, false
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	if len(parts) < 2 {
		return "", 0, false
	}

	idx, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}
	return parts[0], idx, true
}
