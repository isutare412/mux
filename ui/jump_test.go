package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lunemis/mux/tmux"
)

func TestAssignLabels_LabelsWindowsOnly(t *testing.T) {
	items := []listItem{
		{kind: itemSession},
		{kind: itemWindow},
		{kind: itemPane},
		{kind: itemWindow},
	}
	got := assignLabels(items)
	want := []string{"", "a", "", "s"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("labels[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAssignLabels_Overflow(t *testing.T) {
	n := len(jumpAlphabet) + 2
	items := make([]listItem, n)
	for i := range items {
		items[i] = listItem{kind: itemWindow}
	}
	got := assignLabels(items)
	if got[len(jumpAlphabet)-1] == "" {
		t.Errorf("last in-range row should have a label")
	}
	if got[len(jumpAlphabet)] != "" {
		t.Errorf("overflow row should have empty label, got %q", got[len(jumpAlphabet)])
	}
}

func TestRebuildItemsAssignsLabels(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}}})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim"},
		{Index: 1, Name: "claude"},
	}})

	if len(m.labels) != len(m.items) {
		t.Fatalf("labels len %d != items len %d", len(m.labels), len(m.items))
	}
	// items: [session mux, window 0, window 1] -> labels: ["", "a", "s"]
	if m.items[0].kind != itemSession || m.labels[0] != "" {
		t.Errorf("session row should be unlabelled, got kind=%v label=%q", m.items[0].kind, m.labels[0])
	}
	if m.items[1].kind != itemWindow || m.labels[1] != "a" {
		t.Errorf("first window label = %q, want \"a\"", m.labels[1])
	}
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestJumpModeEnterAndJump(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}}})
	m = drive(m, windowsLoadedMsg{sessionName: "mux", windows: []tmux.Window{
		{Index: 0, Name: "nvim"},
		{Index: 1, Name: "claude"},
	}})
	// items: [session mux, window 0, window 1] -> labels: ["", "a", "s"]

	m = drive(m, runeKey('s')) // enter jump mode
	if m.mode != modeJump {
		t.Fatalf("mode = %v, want modeJump", m.mode)
	}

	m = drive(m, runeKey('s')) // 's' is the label for window 1
	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList after jump", m.mode)
	}
	it := m.currentItem()
	if it == nil || it.kind != itemWindow || it.window.Index != 1 {
		t.Fatalf("cursor not on window 1; got %+v", it)
	}
	if (m.attachTarget != previewKey{}) {
		t.Errorf("jump must not set attachTarget, got %+v", m.attachTarget)
	}
}

func TestJumpModeEscCancels(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "eval"}}})
	start := m.cursor

	m = drive(m, runeKey('s'))
	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList", m.mode)
	}
	if m.cursor != start {
		t.Errorf("cursor moved on esc: %d -> %d", start, m.cursor)
	}
}

func TestJumpModeNonLabelCancels(t *testing.T) {
	m := NewModel()
	m = drive(m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "mux"}, {Name: "eval"}}})

	m = drive(m, runeKey('s'))
	m = drive(m, runeKey('q')) // 'q' is in the alphabet but unassigned (no window rows), so cancels

	if m.mode != modeList {
		t.Fatalf("mode = %v, want modeList after non-label key", m.mode)
	}
}

func TestRenderHelp_JumpModeHint(t *testing.T) {
	help := renderHelp(modeJump)
	if !strings.Contains(help, "jump") {
		t.Errorf("jump-mode help should mention jump: %q", help)
	}
	if !strings.Contains(help, "cancel") {
		t.Errorf("jump-mode help should mention cancel: %q", help)
	}
}

func TestRenderHelp_ListModeShowsJumpKey(t *testing.T) {
	help := renderHelp(modeList)
	if !strings.Contains(help, "jump") {
		t.Errorf("list-mode help should advertise jump: %q", help)
	}
}

func TestJumpAlphabetIsFullDistinctAZ(t *testing.T) {
	if len(jumpAlphabet) != 26 {
		t.Fatalf("jumpAlphabet len = %d, want 26", len(jumpAlphabet))
	}
	seen := map[rune]bool{}
	for _, r := range jumpAlphabet {
		if r < 'a' || r > 'z' {
			t.Errorf("non a-z rune %q in jumpAlphabet", r)
		}
		if seen[r] {
			t.Errorf("duplicate rune %q in jumpAlphabet", r)
		}
		seen[r] = true
	}
	if len(seen) != 26 {
		t.Errorf("distinct letters = %d, want 26", len(seen))
	}
}
