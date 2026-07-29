package tmux

import (
	"fmt"
	"testing"
)

func TestLastTarget_UsesClientPointer(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("mux|dotfiles\n"), nil, "tmux", "display-message", "-t", "%2", "-p", lastTargetFormat)
		m.OnOutput([]byte("1\n"), nil, "tmux", "display-message", "-t", "dotfiles", "-p", windowIndexFormat)

		session, window, ok := LastTarget()
		if !ok {
			t.Fatal("expected ok=true")
		}
		if session != "dotfiles" {
			t.Errorf("session = %q, want \"dotfiles\"", session)
		}
		if window != 1 {
			t.Errorf("window = %d, want 1", window)
		}
	})
}

// A display-popup launch leaves $TMUX_PANE empty, so the client query must run
// without -t, mirroring CurrentContext.
func TestLastTarget_PopupLaunch(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "")
		m.OnOutput([]byte("mux|dotfiles\n"), nil, "tmux", "display-message", "-p", lastTargetFormat)
		m.OnOutput([]byte("3\n"), nil, "tmux", "display-message", "-t", "dotfiles", "-p", windowIndexFormat)

		session, window, ok := LastTarget()
		if !ok || session != "dotfiles" || window != 3 {
			t.Fatalf("got (%q, %d, %v), want (\"dotfiles\", 3, true)", session, window, ok)
		}
	})
}

func TestLastTarget_EmptyPointerFallsBackToRecency(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("mux|\n"), nil, "tmux", "display-message", "-t", "%2", "-p", lastTargetFormat)
		m.OnOutput([]byte("common|100\ndotfiles|300\nmux|500\n"), nil, "tmux", "list-sessions", "-F", lastAttachedFormat)
		m.OnOutput([]byte("2\n"), nil, "tmux", "display-message", "-t", "dotfiles", "-p", windowIndexFormat)

		session, window, ok := LastTarget()
		if !ok || session != "dotfiles" || window != 2 {
			t.Fatalf("got (%q, %d, %v), want (\"dotfiles\", 2, true)", session, window, ok)
		}
	})
}

// tmux can report the current session as the last one; that must not produce a
// jump that goes nowhere.
func TestLastTarget_PointerEqualToCurrentFallsBack(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("mux|mux\n"), nil, "tmux", "display-message", "-t", "%2", "-p", lastTargetFormat)
		m.OnOutput([]byte("common|100\nmux|500\n"), nil, "tmux", "list-sessions", "-F", lastAttachedFormat)
		m.OnOutput([]byte("0\n"), nil, "tmux", "display-message", "-t", "common", "-p", windowIndexFormat)

		session, _, ok := LastTarget()
		if !ok || session != "common" {
			t.Fatalf("got (%q, %v), want (\"common\", true)", session, ok)
		}
	})
}

// A killed last session needs no existence check: its window query fails and
// resolution falls through to recency.
func TestLastTarget_DeadPointerFallsBack(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("mux|gone\n"), nil, "tmux", "display-message", "-t", "%2", "-p", lastTargetFormat)
		m.OnOutput(nil, fmt.Errorf("can't find session gone"), "tmux", "display-message", "-t", "gone", "-p", windowIndexFormat)
		m.OnOutput([]byte("common|100\nmux|500\n"), nil, "tmux", "list-sessions", "-F", lastAttachedFormat)
		m.OnOutput([]byte("4\n"), nil, "tmux", "display-message", "-t", "common", "-p", windowIndexFormat)

		session, window, ok := LastTarget()
		if !ok || session != "common" || window != 4 {
			t.Fatalf("got (%q, %d, %v), want (\"common\", 4, true)", session, window, ok)
		}
	})
}

// When the client query fails, clientLastSession returns ("", ""). The empty
// current session must not be treated as "no session to exclude" -- that would
// let the recency fallback select the current session itself as the jump
// target. Bail immediately; list-sessions must never run.
func TestLastTarget_ClientQueryErrorBailsWithoutFallback(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput(nil, fmt.Errorf("no client"), "tmux", "display-message", "-t", "%2", "-p", lastTargetFormat)

		if _, _, ok := LastTarget(); ok {
			t.Error("expected ok=false when the client query errors")
		}
		want := []string{"tmux display-message -t %2 -p " + lastTargetFormat}
		if len(m.outCalls) != len(want) || m.outCalls[0] != want[0] {
			t.Errorf("outCalls = %v, want %v (list-sessions must not run)", m.outCalls, want)
		}
	})
}

func TestLastTarget_NotInTmuxRunsNoCommands(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", "")
		t.Setenv("TMUX_PANE", "")

		if _, _, ok := LastTarget(); ok {
			t.Error("expected ok=false when not running inside tmux")
		}
		if len(m.outCalls) != 0 {
			t.Errorf("expected no commands, got %v", m.outCalls)
		}
	})
}

func TestLastTarget_NoOtherSession(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("mux|\n"), nil, "tmux", "display-message", "-t", "%2", "-p", lastTargetFormat)
		m.OnOutput([]byte("mux|500\n"), nil, "tmux", "list-sessions", "-F", lastAttachedFormat)

		if _, _, ok := LastTarget(); ok {
			t.Error("expected ok=false when no other session exists")
		}
	})
}

// Ties break toward the first line tmux reports.
func TestLastTarget_RecencyTieBreaksToFirstLine(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("mux|\n"), nil, "tmux", "display-message", "-t", "%2", "-p", lastTargetFormat)
		m.OnOutput([]byte("alpha|300\nbeta|300\nmux|500\n"), nil, "tmux", "list-sessions", "-F", lastAttachedFormat)
		m.OnOutput([]byte("0\n"), nil, "tmux", "display-message", "-t", "alpha", "-p", windowIndexFormat)

		session, _, ok := LastTarget()
		if !ok || session != "alpha" {
			t.Fatalf("got (%q, %v), want (\"alpha\", true)", session, ok)
		}
	})
}
