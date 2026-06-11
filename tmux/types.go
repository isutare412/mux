// Package tmux provides functions for managing tmux sessions,
// capturing pane output, and detecting running processes.
package tmux

import "time"

// Session represents a tmux session with its metadata and state.
type Session struct {
	Name          string
	WindowCount   int      // total window count reported by list-sessions
	Windows       []Window // nil until enumerated via ListWindows
	Created       time.Time
	Activity      time.Time
	Attached      bool
	Directory     string
	ActiveCommand string
	PanePID       int
	GitBranch     string // current git branch, empty if not a git repo
	IsWorktree    bool   // true if Directory is a linked git worktree
}

// Window represents a single tmux window inside a session.
type Window struct {
	Index  int
	Name   string
	Active bool
	Panes  []Pane // nil until enumerated via ListPanes
}

// Pane represents a single tmux pane inside a window.
type Pane struct {
	Index   int
	Command string
	Active  bool
	Width   int
	Height  int
	PID     int // pane_pid from list-panes
}

// ClaudeState is the high-level activity state of a Claude Code pane.
type ClaudeState int

const (
	ClaudeNone    ClaudeState = iota // not a Claude pane / no session file
	ClaudeWorking                    // actively generating
	ClaudeWaiting                    // awaiting a permission/tool approval
	ClaudeIdle                       // done / idle
)

// ClaudeInfo is the per-pane Claude snapshot rendered inline in the tree.
type ClaudeInfo struct {
	State ClaudeState
	Recap string    // cleaned recap: ai-title → away_summary → last assistant text, may be ""
	Since time.Time // session updatedAt; elapsed = now - Since
}
