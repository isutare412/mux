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
	got := assignLabels(items, "", 0)
	want := []string{"", "a", "", "d"}
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
	got := assignLabels(items, "", 0)
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
	// items: [session mux, window 0, window 1] -> labels: ["", "a", "d"]
	if m.items[0].kind != itemSession || m.labels[0] != "" {
		t.Errorf("session row should be unlabelled, got kind=%v label=%q", m.items[0].kind, m.labels[0])
	}
	if m.items[1].kind != itemWindow || m.labels[1] != "a" {
		t.Errorf("first window label = %q, want \"a\"", m.labels[1])
	}
	if m.items[2].kind != itemWindow || m.labels[2] != "d" {
		t.Errorf("second window label = %q, want \"d\"", m.labels[2])
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
	// items: [session mux, window 0, window 1] -> labels: ["", "a", "d"]

	m = drive(m, runeKey('s')) // enter jump mode
	if m.mode != modeJump {
		t.Fatalf("mode = %v, want modeJump", m.mode)
	}

	m = drive(m, runeKey('d')) // 'd' is the label for window 1
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

func TestJumpAlphabetExcludesReservedKey(t *testing.T) {
	if len(jumpAlphabet) != 25 {
		t.Fatalf("jumpAlphabet len = %d, want 25 (a-z minus the reserved key)", len(jumpAlphabet))
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
	if seen[jumpReserved] {
		t.Errorf("jumpAlphabet must not contain the reserved key %q", jumpReserved)
	}
	if len(seen) != 25 {
		t.Errorf("distinct letters = %d, want 25", len(seen))
	}
}

func TestAssignLabels_ReservesSForLastTarget(t *testing.T) {
	sessions := []tmux.Session{{Name: "mux"}, {Name: "dotfiles"}}
	windows := []tmux.Window{{Index: 0}, {Index: 1}}
	items := []listItem{
		{kind: itemSession, session: &sessions[0]},
		{kind: itemWindow, session: &sessions[0], window: &windows[0]},
		{kind: itemWindow, session: &sessions[0], window: &windows[1]},
		{kind: itemSession, session: &sessions[1]},
		{kind: itemWindow, session: &sessions[1], window: &windows[0]},
	}

	got := assignLabels(items, "dotfiles", 0)
	// The target takes "s" without consuming a pool letter, so the other
	// window rows keep the labels they would have had anyway.
	want := []string{"", "a", "d", "", "s"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("labels[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAssignLabels_NoRowGetsSWithoutATarget(t *testing.T) {
	sessions := []tmux.Session{{Name: "mux"}}
	windows := []tmux.Window{{Index: 0}, {Index: 1}}
	items := []listItem{
		{kind: itemSession, session: &sessions[0]},
		{kind: itemWindow, session: &sessions[0], window: &windows[0]},
		{kind: itemWindow, session: &sessions[0], window: &windows[1]},
	}

	for i, label := range assignLabels(items, "", 0) {
		if label == string(jumpReserved) {
			t.Errorf("labels[%d] = %q, want no reserved label without a target", i, label)
		}
	}
}

func TestLastTargetMsgStoresTarget(t *testing.T) {
	m := NewModel()
	m = drive(m, lastTargetMsg{session: "dotfiles", window: 2, ok: true})

	if m.lastSession != "dotfiles" {
		t.Errorf("lastSession = %q, want \"dotfiles\"", m.lastSession)
	}
	if m.lastWindow != 2 {
		t.Errorf("lastWindow = %d, want 2", m.lastWindow)
	}
}

func TestLastTargetMsgIgnoredWhenNotOk(t *testing.T) {
	m := NewModel()
	m = drive(m, lastTargetMsg{session: "dotfiles", window: 2, ok: false})

	if m.lastSession != "" {
		t.Errorf("lastSession = %q, want empty when ok=false", m.lastSession)
	}
}
