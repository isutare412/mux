package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lunemis/mux/tmux"
)

type renameModel struct {
	input       textinput.Model
	kind        itemKind // itemSession or itemWindow
	oldName     string
	sessionName string // parent session (set for window renames)
	windowIndex int    // tmux window index (set for window renames)
	err         error
}

type sessionRenamedMsg struct {
	oldName string
	newName string
}

type windowRenamedMsg struct {
	sessionName string
}

func newRenameInput(oldName string) textinput.Model {
	input := textinput.New()
	input.Placeholder = oldName
	input.SetValue(oldName)
	input.Focus()
	input.CharLimit = 50
	input.Width = 40
	return input
}

// newRenameModel builds a model for renaming a session.
func newRenameModel(oldName string) renameModel {
	return renameModel{
		input:   newRenameInput(oldName),
		kind:    itemSession,
		oldName: oldName,
	}
}

// newWindowRenameModel builds a model for renaming a window within a session.
func newWindowRenameModel(sessionName string, windowIndex int, oldName string) renameModel {
	return renameModel{
		input:       newRenameInput(oldName),
		kind:        itemWindow,
		oldName:     oldName,
		sessionName: sessionName,
		windowIndex: windowIndex,
	}
}

func (m renameModel) Update(msg tea.Msg) (renameModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			newName := m.input.Value()
			if newName == "" || newName == m.oldName {
				return m, nil
			}
			if m.kind == itemWindow {
				if err := tmux.RenameWindow(m.sessionName, m.windowIndex, newName); err != nil {
					m.err = err
					return m, nil
				}
				sessionName := m.sessionName
				return m, func() tea.Msg {
					return windowRenamedMsg{sessionName: sessionName}
				}
			}
			if err := tmux.RenameSession(m.oldName, newName); err != nil {
				m.err = err
				return m, nil
			}
			return m, func() tea.Msg {
				return sessionRenamedMsg{oldName: m.oldName, newName: newName}
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m renameModel) View() string {
	title := "Rename Session"
	if m.kind == itemWindow {
		title = "Rename Window"
	}
	s := inputLabelStyle.Render(title) + "\n\n"
	s += inputLabelStyle.Render("Name: ") + m.input.View() + "\n\n"
	s += helpStyle.Render("enter confirm • esc cancel")

	if m.err != nil {
		s += "\n" + errorStyle.Render(m.err.Error())
	}

	return s
}
