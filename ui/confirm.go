package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lunemis/mux/tmux"
)

// killTarget identifies what a confirm-kill prompt will destroy. For window and
// pane targets, session holds the parent session name.
type killTarget struct {
	kind        itemKind
	session     string
	windowIndex int
	paneIndex   int
	label       string // shown in the prompt
}

type confirmKillModel struct {
	target killTarget
}

// killedMsg reports the outcome of a confirm-kill prompt. done is false when the
// user cancelled. session carries the parent session so the refresh handler
// knows which windows to reload.
type killedMsg struct {
	kind    itemKind
	session string
	err     error
	done    bool
}

func newConfirmKillModel(target killTarget) confirmKillModel {
	return confirmKillModel{target: target}
}

func (m confirmKillModel) Update(msg tea.Msg) (confirmKillModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			t := m.target
			var err error
			switch t.kind {
			case itemWindow:
				err = tmux.KillWindow(t.session, t.windowIndex)
			case itemPane:
				err = tmux.KillPane(t.session, t.windowIndex, t.paneIndex)
			default:
				err = tmux.KillSession(t.session)
			}
			return m, func() tea.Msg {
				return killedMsg{kind: t.kind, session: t.session, err: err, done: true}
			}
		default:
			// Any other key cancels.
			return m, func() tea.Msg {
				return killedMsg{done: false}
			}
		}
	}
	return m, nil
}

func (m confirmKillModel) View() string {
	return errorStyle.Render(
		fmt.Sprintf("Kill %s %q? (y/N)", killKindLabel(m.target.kind), m.target.label))
}

func killKindLabel(k itemKind) string {
	switch k {
	case itemWindow:
		return "window"
	case itemPane:
		return "pane"
	default:
		return "session"
	}
}

// killTargetForItem builds the kill target for the row under the cursor.
func killTargetForItem(it listItem) killTarget {
	switch it.kind {
	case itemWindow:
		return killTarget{
			kind:        itemWindow,
			session:     it.session.Name,
			windowIndex: it.window.Index,
			label:       fmt.Sprintf("%d:%s", it.window.Index, it.window.Name),
		}
	case itemPane:
		return killTarget{
			kind:        itemPane,
			session:     it.session.Name,
			windowIndex: it.window.Index,
			paneIndex:   it.pane.Index,
			label:       fmt.Sprintf("%d:%s", it.pane.Index, it.pane.Command),
		}
	default:
		return killTarget{
			kind:    itemSession,
			session: it.session.Name,
			label:   it.session.Name,
		}
	}
}
