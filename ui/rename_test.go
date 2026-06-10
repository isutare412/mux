package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lunemis/mux/tmux"
)

// recordingRunner captures Run invocations so tests can assert the tmux
// command that a model dispatched.
type recordingRunner struct {
	runs []string
}

func (r *recordingRunner) Output(name string, args ...string) ([]byte, error) {
	return nil, nil
}

func (r *recordingRunner) Run(name string, args ...string) error {
	r.runs = append(r.runs, name+" "+strings.Join(args, " "))
	return nil
}

// pressEnter sends an Enter key to the rename model and returns the model and
// the message produced by its returned command (nil if no command).
func pressEnter(m renameModel) (renameModel, tea.Msg) {
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

func TestWindowRename_DispatchesRenameWindow(t *testing.T) {
	rec := &recordingRunner{}
	tmux.SetRunner(rec)

	m := newWindowRenameModel("dev", 2, "old")
	m.input.SetValue("logs")

	_, msg := pressEnter(m)

	wmsg, ok := msg.(windowRenamedMsg)
	if !ok {
		t.Fatalf("expected windowRenamedMsg, got %T (%v)", msg, msg)
	}
	if wmsg.sessionName != "dev" {
		t.Errorf("windowRenamedMsg.sessionName = %q, want dev", wmsg.sessionName)
	}
	if len(rec.runs) != 1 {
		t.Fatalf("expected 1 tmux call, got %d (%v)", len(rec.runs), rec.runs)
	}
	want := "tmux rename-window -t dev:2 logs"
	if rec.runs[0] != want {
		t.Errorf("tmux call = %q, want %q", rec.runs[0], want)
	}
}
