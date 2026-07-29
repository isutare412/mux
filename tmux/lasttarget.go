package tmux

import (
	"os"
	"strconv"
	"strings"
)

// lastTargetFormat asks for the current session name and the client's previous
// session in a single display-message call.
const lastTargetFormat = "#{session_name}|#{client_last_session}"

// lastAttachedFormat lists every session with the time it was last attached,
// used to pick a fallback when tmux's last-session pointer is unusable.
const lastAttachedFormat = "#{session_name}|#{session_last_attached}"

// windowIndexFormat resolves a session's active window index.
const windowIndexFormat = "#{window_index}"

// LastTarget returns the session name and active window index of the session
// the user was in before the current one — the destination of jump mode's
// reserved `s` label. ok is false when mux is not running inside tmux, or when
// no other session can be resolved.
//
// tmux's own pointer (#{client_last_session}) is preferred. When it is empty,
// names the current session, or names a session that no longer exists, the most
// recently attached other session is used instead. A dead pointer needs no
// explicit existence check: resolving its window fails and falls through.
func LastTarget() (session string, window int, ok bool) {
	if os.Getenv("TMUX") == "" {
		return "", 0, false
	}

	current, last := clientLastSession()
	if last != "" && last != current {
		if idx, found := activeWindowIndex(last); found {
			return last, idx, true
		}
	}

	fallback := mostRecentlyAttached(current)
	if fallback == "" {
		return "", 0, false
	}
	idx, found := activeWindowIndex(fallback)
	if !found {
		return "", 0, false
	}
	return fallback, idx, true
}

// clientLastSession returns the client's current session name and its previous
// session name. Either may be empty when tmux cannot resolve them.
//
// A direct launch sets $TMUX_PANE, which we target explicitly. A display-popup
// launch leaves it empty, so we fall back to display-message without -t, which
// resolves against the client that opened the popup.
func clientLastSession() (current, last string) {
	args := []string{"display-message", "-p", lastTargetFormat}
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		args = []string{"display-message", "-t", pane, "-p", lastTargetFormat}
	}

	out, err := runner.Output("tmux", args...)
	if err != nil {
		return "", ""
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	if len(parts) < 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// activeWindowIndex returns the index of the given session's active window.
// ok is false when the session does not exist or the index does not parse.
func activeWindowIndex(session string) (int, bool) {
	out, err := runner.Output("tmux", "display-message", "-t", session, "-p", windowIndexFormat)
	if err != nil {
		return 0, false
	}
	idx, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, false
	}
	return idx, true
}

// mostRecentlyAttached returns the name of the session with the newest
// session_last_attached timestamp, excluding current. Returns "" when there is
// no other session. Ties break toward the first line tmux reports.
func mostRecentlyAttached(current string) string {
	out, err := runner.Output("tmux", "list-sessions", "-F", lastAttachedFormat)
	if err != nil {
		return ""
	}

	best := ""
	bestAt := int64(-1)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) < 2 || parts[0] == "" || parts[0] == current {
			continue
		}
		at, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		if at > bestAt {
			best, bestAt = parts[0], at
		}
	}
	return best
}
