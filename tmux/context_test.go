package tmux

import (
	"fmt"
	"testing"
)

// fakeTmux is a representative value of the $TMUX env var set inside a tmux client.
const fakeTmux = "/tmp/tmux-501/default,1234,0"

func TestCurrentContext_DirectLaunch(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("mux|1\n"), nil, "tmux", "display-message", "-t", "%2", "-p", currentContextFormat)

		session, window, ok := CurrentContext()
		if !ok {
			t.Fatal("expected ok=true")
		}
		if session != "mux" {
			t.Errorf("session = %q, want \"mux\"", session)
		}
		if window != 1 {
			t.Errorf("window = %d, want 1", window)
		}
	})
}

// A display-popup launch leaves $TMUX_PANE empty but $TMUX set. CurrentContext
// must fall back to `display-message` without -t, which resolves the client's
// current pane (the window the popup was opened from).
func TestCurrentContext_PopupLaunch(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "")
		m.OnOutput([]byte("mux|2\n"), nil, "tmux", "display-message", "-p", currentContextFormat)

		session, window, ok := CurrentContext()
		if !ok {
			t.Fatal("expected ok=true for popup launch")
		}
		if session != "mux" {
			t.Errorf("session = %q, want \"mux\"", session)
		}
		if window != 2 {
			t.Errorf("window = %d, want 2", window)
		}
	})
}

func TestCurrentContext_NotInTmux(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", "")
		t.Setenv("TMUX_PANE", "")
		if _, _, ok := CurrentContext(); ok {
			t.Error("expected ok=false when not running inside tmux")
		}
	})
}

func TestCurrentContext_RunnerError(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput(nil, fmt.Errorf("no server"), "tmux", "display-message", "-t", "%2", "-p", currentContextFormat)
		if _, _, ok := CurrentContext(); ok {
			t.Error("expected ok=false on runner error")
		}
	})
}

func TestCurrentContext_MalformedOutput(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("mux\n"), nil, "tmux", "display-message", "-t", "%2", "-p", currentContextFormat)
		if _, _, ok := CurrentContext(); ok {
			t.Error("expected ok=false on missing window field")
		}
	})
}

func TestCurrentContext_NonIntegerWindowIndex(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("mux|abc\n"), nil, "tmux", "display-message", "-t", "%2", "-p", currentContextFormat)
		if _, _, ok := CurrentContext(); ok {
			t.Error("expected ok=false on non-integer window index")
		}
	})
}

func TestCurrentContext_EmptySessionName(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX", fakeTmux)
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("|3\n"), nil, "tmux", "display-message", "-t", "%2", "-p", currentContextFormat)
		if _, _, ok := CurrentContext(); ok {
			t.Error("expected ok=false when session name is empty")
		}
	})
}
