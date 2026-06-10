package tmux

import (
	"fmt"
	"testing"
)

func TestCurrentContext_Success(t *testing.T) {
	withMock(t, func(m *mockRunner) {
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

func TestCurrentContext_NoPaneEnv(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX_PANE", "")
		if _, _, ok := CurrentContext(); ok {
			t.Error("expected ok=false when TMUX_PANE is unset")
		}
	})
}

func TestCurrentContext_RunnerError(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput(nil, fmt.Errorf("no server"), "tmux", "display-message", "-t", "%2", "-p", currentContextFormat)
		if _, _, ok := CurrentContext(); ok {
			t.Error("expected ok=false on runner error")
		}
	})
}

func TestCurrentContext_MalformedOutput(t *testing.T) {
	withMock(t, func(m *mockRunner) {
		t.Setenv("TMUX_PANE", "%2")
		m.OnOutput([]byte("mux\n"), nil, "tmux", "display-message", "-t", "%2", "-p", currentContextFormat)
		if _, _, ok := CurrentContext(); ok {
			t.Error("expected ok=false on missing window field")
		}
	})
}
