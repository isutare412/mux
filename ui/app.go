// Package ui implements the Bubble Tea TUI for browsing and managing tmux sessions.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lunemis/mux/tmux"
)

const (
	// Layout
	listWidthPercent = 2  // numerator of 5 (40%)
	listWidthDenom   = 5  // denominator
	minPanelHeight   = 5

	// Timing
	refreshInterval = 500 * time.Millisecond

	// Display limits
	maxSessionNameDisplay = 18
	maxPathDisplay        = 35
	filterCharLimit       = 50
	filterInputWidth      = 30
)

type mode int

const (
	modeList mode = iota
	modeCreate
	modeRename
	modeFilter
	modeConfirmKill
	modeJump
)

// Model is the top-level Bubble Tea model for the session manager TUI.
type Model struct {
	sessions       []tmux.Session
	filtered       []tmux.Session
	items          []listItem // flattened tree of (sessions, windows, panes)
	labels         []string   // jump labels, parallel to items ("" = no label)
	tree           treeState
	cursor         int
	mode           mode
	width          int
	height         int
	err            error
	createModel      createModel
	renameModel      renameModel
	filterMod        filterModel
	confirmKillMod   confirmKillModel
	filterText       string
	attachTarget     previewKey // set when we want to attach after quitting (zero value = no attach)
	focusSession     string // session name to focus cursor on after next load
	focusWindow      int    // window index to focus within focusSession; -1 = session row. Only meaningful when focusSession != "".
	lastSession      string // reserved-jump target session ("" = none)
	lastWindow       int    // active window index within lastSession
	previewContent string           // cached capture-pane output
	previewKey     previewKey       // (session, window, pane) the cache belongs to
	tokenUsage     *tmux.TokenUsage // cached token usage for current AI session
	tokenSession   string           // session name the token cache belongs to
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type sessionsLoadedMsg struct {
	sessions []tmux.Session
	err      error
}

func loadSessions() tea.Msg {
	sessions, err := tmux.ListSessions()
	return sessionsLoadedMsg{sessions: sessions, err: err}
}

type previewLoadedMsg struct {
	key     previewKey
	content string
}

type tokenUsageLoadedMsg struct {
	sessionName string
	usage       *tmux.TokenUsage
}

type windowsLoadedMsg struct {
	sessionName string
	windows     []tmux.Window
}

type panesLoadedMsg struct {
	sessionName string
	windowIndex int
	panes       []tmux.Pane
}

type currentContextMsg struct {
	session string
	window  int
	ok      bool
}

func loadCurrentContext() tea.Msg {
	session, window, ok := tmux.CurrentContext()
	return currentContextMsg{session: session, window: window, ok: ok}
}

type lastTargetMsg struct {
	session string
	window  int
	ok      bool
}

// loadLastTarget resolves jump mode's reserved `s` destination. It runs once
// from Init, never on the refresh tick — the pointer only changes when the user
// switches sessions, which cannot happen while mux is on screen.
func loadLastTarget() tea.Msg {
	session, window, ok := tmux.LastTarget()
	return lastTargetMsg{session: session, window: window, ok: ok}
}

func loadWindows(sessionName string) tea.Cmd {
	return func() tea.Msg {
		windows, _ := tmux.ListWindows(sessionName)
		return windowsLoadedMsg{sessionName: sessionName, windows: windows}
	}
}

func loadPanes(sessionName string, windowIndex int) tea.Cmd {
	return func() tea.Msg {
		panes, _ := tmux.ListPanes(sessionName, windowIndex)
		return panesLoadedMsg{sessionName: sessionName, windowIndex: windowIndex, panes: panes}
	}
}

func refreshPreview(key previewKey) tea.Cmd {
	return func() tea.Msg {
		content, err := tmux.CapturePaneTarget(key.target())
		if err != nil {
			content = "Error: " + err.Error()
		}
		return previewLoadedMsg{key: key, content: content}
	}
}

func loadTokenUsage(sessionName string, panePID int) tea.Cmd {
	return func() tea.Msg {
		sessionID, cwd, configDir, err := tmux.FindClaudeSession(panePID)
		if err != nil {
			return tokenUsageLoadedMsg{sessionName: sessionName}
		}
		usage, _ := tmux.LoadTokenUsage(sessionID, cwd, configDir)
		return tokenUsageLoadedMsg{sessionName: sessionName, usage: usage}
	}
}

type claudeInfoLoadedMsg struct {
	panePID int
	info    *tmux.ClaudeInfo
}

func loadClaudeInfo(panePID int) tea.Cmd {
	return func() tea.Msg {
		info, err := tmux.LoadClaudeInfo(panePID)
		if err != nil {
			return claudeInfoLoadedMsg{panePID: panePID, info: nil}
		}
		return claudeInfoLoadedMsg{panePID: panePID, info: info}
	}
}

// NewModel returns a new Model with default settings.
func NewModel() Model {
	return Model{tree: newTreeState(), focusWindow: -1}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadSessions, loadCurrentContext, loadLastTarget, tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		cmds := []tea.Cmd{loadSessions, tick()}
		if it := m.currentItem(); it != nil {
			cmds = append(cmds, refreshPreview(previewKeyForItem(*it)))
			if tmux.IsAICommand(it.session.ActiveCommand) {
				cmds = append(cmds, loadTokenUsage(it.session.Name, it.session.PanePID))
			}
		}
		// Refresh windows for expanded subtrees.
		for name := range m.tree.expandedSession {
			cmds = append(cmds, loadWindows(name))
		}
		// For every visible window, ensure its panes are loaded (roll-up needs
		// them even when the window is collapsed), and load Claude info for any
		// visible Claude pane.
		seenWindow := make(map[paneCacheKey]struct{})
		for _, it := range m.items {
			switch it.kind {
			case itemWindow:
				key := paneCacheKey{session: it.session.Name, window: it.window.Index}
				if _, done := seenWindow[key]; !done {
					seenWindow[key] = struct{}{}
					cmds = append(cmds, loadPanes(it.session.Name, it.window.Index))
				}
			case itemPane:
				if it.pane.PID > 0 && tmux.IsAICommand(it.pane.Command) {
					cmds = append(cmds, loadClaudeInfo(it.pane.PID))
				}
			}
		}
		// Claude info for panes of visible (possibly collapsed) windows, for roll-up.
		for key := range seenWindow {
			for _, p := range m.tree.panesCache[key] {
				if p.PID > 0 && tmux.IsAICommand(p.Command) {
					cmds = append(cmds, loadClaudeInfo(p.PID))
				}
			}
		}
		return m, tea.Batch(cmds...)

	case sessionsLoadedMsg:
		m.err = msg.err
		if msg.sessions != nil {
			m.sessions = msg.sessions
			m.tree.pruneCaches(m.sessions)
			// Auto-expand sessions the first time they appear so the tree
			// opens fully on startup (and newly created sessions open too).
			var cmds []tea.Cmd
			for _, name := range m.tree.expandNewSessions(m.sessions) {
				cmds = append(cmds, loadWindows(name))
			}
			m.applyFilter()
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case windowsLoadedMsg:
		m.tree.windowsCache[msg.sessionName] = msg.windows
		m.rebuildItems()
		// Eagerly load each window's panes instead of waiting for the next tick,
		// so Claude state/recap appear promptly on first view.
		var cmds []tea.Cmd
		for _, w := range msg.windows {
			cmds = append(cmds, loadPanes(msg.sessionName, w.Index))
		}
		return m, tea.Batch(cmds...)

	case panesLoadedMsg:
		m.tree.panesCache[paneCacheKey{session: msg.sessionName, window: msg.windowIndex}] = msg.panes
		m.rebuildItems()
		// Eagerly load Claude info for AI panes so the glyph, timer, and recap
		// render without waiting for a subsequent tick.
		var cmds []tea.Cmd
		for _, p := range msg.panes {
			if p.PID > 0 && tmux.IsAICommand(p.Command) {
				cmds = append(cmds, loadClaudeInfo(p.PID))
			}
		}
		return m, tea.Batch(cmds...)

	case currentContextMsg:
		if msg.ok {
			m.focusSession = msg.session
			m.focusWindow = msg.window
			m.rebuildItems()
		}
		return m, nil

	case lastTargetMsg:
		if msg.ok {
			m.lastSession = msg.session
			m.lastWindow = msg.window
			m.rebuildItems()
		}
		return m, nil

	case previewLoadedMsg:
		m.previewKey = msg.key
		m.previewContent = msg.content
		return m, nil

	case tokenUsageLoadedMsg:
		m.tokenSession = msg.sessionName
		m.tokenUsage = msg.usage
		return m, nil

	case claudeInfoLoadedMsg:
		if msg.info != nil {
			m.tree.claudeCache[msg.panePID] = *msg.info
		} else {
			delete(m.tree.claudeCache, msg.panePID)
		}
		return m, nil

	case sessionCreatedMsg:
		m.mode = modeList
		m.focusSession = msg.name
		m.focusWindow = -1
		return m, loadSessions

	case sessionRenamedMsg:
		m.mode = modeList
		return m, loadSessions

	case windowRenamedMsg:
		m.mode = modeList
		return m, loadWindows(msg.sessionName)

	case filterAppliedMsg:
		m.mode = modeList
		m.filterText = msg.text
		m.applyFilter()
		return m, nil

	case killedMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.mode = modeList
		if !msg.done {
			return m, nil
		}
		if msg.kind == itemSession {
			return m, loadSessions
		}
		return m, tea.Batch(loadSessions, loadWindows(msg.session))
	}

	switch m.mode {
	case modeCreate:
		return m.updateCreate(msg)
	case modeRename:
		return m.updateRename(msg)
	case modeFilter:
		return m.updateFilter(msg)
	case modeConfirmKill:
		return m.updateConfirmKill(msg)
	case modeJump:
		return m.updateJump(msg)
	default:
		return m.updateList(msg)
	}
}

func (m Model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			m.focusSession = ""
			m.focusWindow = -1
			if m.cursor > 0 {
				m.cursor--
				return m, m.refreshCurrentPreview()
			}
		case "down", "j":
			m.focusSession = ""
			m.focusWindow = -1
			if m.cursor < len(m.items)-1 {
				m.cursor++
				return m, m.refreshCurrentPreview()
			}
		case "g":
			m.focusSession = ""
			m.focusWindow = -1
			m.cursor = 0
			return m, m.refreshCurrentPreview()
		case "G":
			m.focusSession = ""
			m.focusWindow = -1
			if len(m.items) > 0 {
				m.cursor = len(m.items) - 1
				return m, m.refreshCurrentPreview()
			}

		case "J":
			m.focusSession = ""
			m.focusWindow = -1
			if idx, ok := nextSessionStop(m.items, m.cursor, 1); ok {
				m.cursor = idx
				return m, m.refreshCurrentPreview()
			}
		case "K":
			m.focusSession = ""
			m.focusWindow = -1
			if idx, ok := nextSessionStop(m.items, m.cursor, -1); ok {
				m.cursor = idx
				return m, m.refreshCurrentPreview()
			}

		case "tab", "right", "l":
			return m.expandCurrent()

		case "shift+tab", "left", "h":
			return m.collapseCurrent()

		case "enter":
			if it := m.currentItem(); it != nil {
				m.attachTarget = previewKeyForItem(*it)
				return m, tea.Quit
			}

		case "n":
			m.mode = modeCreate
			m.createModel = newCreateModel()
			return m, m.createModel.nameInput.Focus()

		case "x":
			if it := m.currentItem(); it != nil {
				m.mode = modeConfirmKill
				m.confirmKillMod = newConfirmKillModel(killTargetForItem(*it))
			}

		case "r":
			if it := m.currentItem(); it != nil {
				switch it.kind {
				case itemSession:
					m.mode = modeRename
					m.renameModel = newRenameModel(it.session.Name)
					return m, m.renameModel.input.Focus()
				case itemWindow:
					m.mode = modeRename
					m.renameModel = newWindowRenameModel(it.session.Name, it.window.Index, it.window.Name)
					return m, m.renameModel.input.Focus()
				}
			}

		case "s":
			m.mode = modeJump
			return m, nil

		case "/":
			m.mode = modeFilter
			m.filterMod = newFilterModel(m.filterText)
			return m, nil

		case "esc":
			if m.filterText != "" {
				m.filterText = ""
				m.applyFilter()
			}
		}
	}
	return m, nil
}

// updateJump handles keys while jump mode is active. A label key moves the
// cursor to that row and exits to list mode (it does NOT attach); the reserved
// key jumps to the last session's active window, expanding it if needed; esc or
// any other key cancels jump mode.
func (m Model) updateJump(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "esc" {
		m.mode = modeList
		return m, nil
	}
	if key.String() == string(jumpReserved) {
		m.mode = modeList
		if m.lastSession == "" {
			return m, nil
		}
		// The target's session row is missing only when the filter hides it —
		// every session in the filtered list contributes a row regardless of
		// expansion. Bail before expanding or arming a focus: the jump moves the
		// cursor now or not at all, never later when the filter is cleared.
		if m.findItemIndex(itemSession, m.lastSession, 0, 0) < 0 {
			return m, nil
		}
		var cmds []tea.Cmd
		if !m.tree.isSessionExpanded(m.lastSession) {
			m.tree.setSessionExpanded(m.lastSession, true)
			cmds = append(cmds, loadWindows(m.lastSession))
		}
		// Reuse the pending-focus machinery: rebuildItems snaps the cursor when
		// the row is already present, otherwise the target stays pending until
		// windowsLoadedMsg triggers the next rebuild.
		m.focusSession = m.lastSession
		m.focusWindow = m.lastWindow
		m.rebuildItems()
		cmds = append(cmds, m.refreshCurrentPreview())
		return m, tea.Batch(cmds...)
	}
	for i, label := range m.labels {
		if label != "" && label == key.String() {
			m.mode = modeList
			m.focusSession = ""
			m.focusWindow = -1
			m.cursor = i
			return m, m.refreshCurrentPreview()
		}
	}
	// Any other key cancels jump mode.
	m.mode = modeList
	return m, nil
}

// expandCurrent expands the row under the cursor and dispatches the loader.
// On a pane (leaf) it does nothing.
func (m Model) expandCurrent() (tea.Model, tea.Cmd) {
	it := m.currentItem()
	if it == nil || !it.canExpand() {
		return m, nil
	}
	switch it.kind {
	case itemSession:
		if m.tree.isSessionExpanded(it.session.Name) {
			return m, nil
		}
		m.tree.setSessionExpanded(it.session.Name, true)
		m.rebuildItems()
		return m, loadWindows(it.session.Name)
	case itemWindow:
		if m.tree.isWindowExpanded(it.session.Name, it.window.Index) {
			return m, nil
		}
		m.tree.setWindowExpanded(it.session.Name, it.window.Index, true)
		m.rebuildItems()
		return m, loadPanes(it.session.Name, it.window.Index)
	}
	return m, nil
}

// collapseCurrent collapses the row under the cursor. On a child row whose own
// kind cannot collapse further, it walks up to the parent and collapses that.
func (m Model) collapseCurrent() (tea.Model, tea.Cmd) {
	it := m.currentItem()
	if it == nil {
		return m, nil
	}
	switch it.kind {
	case itemSession:
		if !m.tree.isSessionExpanded(it.session.Name) {
			return m, nil
		}
		m.tree.setSessionExpanded(it.session.Name, false)
	case itemWindow:
		if m.tree.isWindowExpanded(it.session.Name, it.window.Index) {
			m.tree.setWindowExpanded(it.session.Name, it.window.Index, false)
		} else {
			// Already-collapsed window: jump up to the parent session
			m.cursor = m.findItemIndex(itemSession, it.session.Name, 0, 0)
			m.tree.setSessionExpanded(it.session.Name, false)
		}
	case itemPane:
		// Collapse the parent window and move cursor up to it
		m.cursor = m.findItemIndex(itemWindow, it.session.Name, it.window.Index, 0)
		m.tree.setWindowExpanded(it.session.Name, it.window.Index, false)
	}
	m.rebuildItems()
	if m.cursor >= len(m.items) {
		m.cursor = max(0, len(m.items)-1)
	}
	return m, m.refreshCurrentPreview()
}

// refreshCurrentPreview returns a tea.Cmd to capture the pane targeted by the
// current cursor position. Returns nil when there is no current item.
func (m *Model) refreshCurrentPreview() tea.Cmd {
	if it := m.currentItem(); it != nil {
		return refreshPreview(previewKeyForItem(*it))
	}
	return nil
}

// findItemIndex returns the index of the matching listItem, or -1 if not found.
func (m *Model) findItemIndex(kind itemKind, sessionName string, windowIdx, paneIdx int) int {
	for i, it := range m.items {
		if it.kind != kind || it.session.Name != sessionName {
			continue
		}
		switch kind {
		case itemSession:
			return i
		case itemWindow:
			if it.window.Index == windowIdx {
				return i
			}
		case itemPane:
			if it.window.Index == windowIdx && it.pane.Index == paneIdx {
				return i
			}
		}
	}
	return -1
}

func (m Model) updateCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.mode = modeList
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.createModel, cmd = m.createModel.Update(msg)
	return m, cmd
}

func (m Model) updateRename(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.mode = modeList
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.renameModel, cmd = m.renameModel.Update(msg)
	return m, cmd
}

func (m Model) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.filterMod, cmd = m.filterMod.Update(msg)
	// Live filter as you type
	m.filterText = m.filterMod.LiveText()
	m.applyFilter()
	return m, cmd
}

func (m Model) updateConfirmKill(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.confirmKillMod, cmd = m.confirmKillMod.Update(msg)
	return m, cmd
}

func (m *Model) currentItem() *listItem {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return &m.items[m.cursor]
	}
	return nil
}

// currentSession returns the parent session of the current row (the row itself
// for session rows). Returns nil if no row is selected.
func (m *Model) currentSession() *tmux.Session {
	if it := m.currentItem(); it != nil {
		return it.session
	}
	return nil
}

func (m *Model) currentSessionName() string {
	if s := m.currentSession(); s != nil {
		return s.Name
	}
	return ""
}

// rebuildItems recomputes the flattened tree view from the filtered session
// list and current expansion state. Call after sessions, filter, or expansion
// state changes.
func (m *Model) rebuildItems() {
	m.items = flatten(m.filtered, &m.tree)
	m.labels = assignLabels(m.items, m.lastSession, m.lastWindow)
	if m.cursor >= len(m.items) {
		m.cursor = max(0, len(m.items)-1)
	}
	m.applyPendingFocus()
}

// applyPendingFocus moves the cursor to the pending focus target if its row is
// present, then clears the target. A window target that hasn't loaded yet stays
// pending so a later rebuild (after windowsLoadedMsg) can snap to it.
func (m *Model) applyPendingFocus() {
	if m.focusSession == "" {
		return
	}
	if m.focusWindow >= 0 {
		if idx := m.findItemIndex(itemWindow, m.focusSession, m.focusWindow, 0); idx >= 0 {
			m.cursor = idx
			m.focusSession = ""
		}
		return
	}
	if idx := m.findItemIndex(itemSession, m.focusSession, 0, 0); idx >= 0 {
		m.cursor = idx
		m.focusSession = ""
	}
}

func (m *Model) applyFilter() {
	if m.filterText == "" {
		m.filtered = m.sessions
	} else {
		lower := strings.ToLower(m.filterText)
		m.filtered = nil
		for _, s := range m.sessions {
			if strings.Contains(strings.ToLower(s.Name), lower) ||
				strings.Contains(strings.ToLower(s.Directory), lower) {
				m.filtered = append(m.filtered, s)
			}
		}
	}
	m.rebuildItems()
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	switch m.mode {
	case modeCreate:
		return m.viewWithOverlay(m.createModel.View())
	case modeRename:
		return m.viewWithOverlay(m.renameModel.View())
	default:
		return m.viewMain()
	}
}

func (m Model) viewMain() string {
	// Title — count sessions only, not windows/panes
	count := fmt.Sprintf("(%d)", len(m.filtered))
	title := titleStyle.Render("⚡ tmux sessions " + count)

	// Help bar
	help := renderHelp(m.mode, m.lastSession != "")

	// Filter / confirm bar
	var extraBar string
	if m.mode == modeFilter {
		extraBar = m.filterMod.View()
	} else if m.mode == modeConfirmKill {
		extraBar = m.confirmKillMod.View()
	} else if m.filterText != "" {
		extraBar = helpStyle.Render(fmt.Sprintf("filter: %s (esc clear)", m.filterText))
	}

	// Chrome: title(1+margin1) + help(1) + extraBar(0 or 1)
	chrome := 3
	if extraBar != "" {
		chrome++
	}

	// Panel height = total height for both borders + content
	panelHeight := m.height - chrome
	if panelHeight < minPanelHeight {
		panelHeight = minPanelHeight
	}

	// Layout: list on left, preview on right
	listWidth := m.width * listWidthPercent / listWidthDenom
	previewWidth := m.width - listWidth

	// Render both panels (each returns exactly panelHeight lines)
	list := renderListView(m.items, m.cursor, m.filterText, &m.tree, listWidth, panelHeight, m.labels, m.mode == modeJump)

	currentItem := m.currentItem()
	currentSession := m.currentSession()
	cachedContent := ""
	if currentItem != nil && m.previewKey == previewKeyForItem(*currentItem) {
		cachedContent = m.previewContent
	}
	var tokenUsage *tmux.TokenUsage
	if currentSession != nil && m.tokenSession == currentSession.Name {
		tokenUsage = m.tokenUsage
	}
	preview := renderPreview(currentItem, cachedContent, previewWidth, panelHeight, tokenUsage)

	// Join line-by-line for exact alignment
	content := joinHorizontalFixed(list, preview)

	// Assemble
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	if extraBar != "" {
		b.WriteString(extraBar)
		b.WriteByte('\n')
	}
	b.WriteString(content)
	b.WriteByte('\n')
	b.WriteString(help)

	return b.String()
}

func (m Model) viewWithOverlay(overlay string) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Render(overlay)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		box)
}

func renderHelp(m mode, hasTarget bool) string {
	if m == modeJump {
		segments := []string{
			helpKeyStyle.Render("a…z") + " " + helpStyle.Render("jump"),
		}
		if hasTarget {
			segments = append(segments, helpKeyStyle.Render("s")+" "+helpStyle.Render("last"))
		}
		segments = append(segments, helpKeyStyle.Render("esc")+" "+helpStyle.Render("cancel"))
		return strings.Join(segments, helpStyle.Render("  •  "))
	}

	keys := []struct{ key, desc string }{
		{"↑↓/jk", "navigate"},
		{"s", "jump"},
		{"tab", "expand"},
		{"⇧tab", "collapse"},
		{"enter", "attach"},
		{"n", "new"},
		{"x", "kill"},
		{"r", "rename"},
		{"/", "filter"},
		{"q", "quit"},
	}

	var parts []string
	for _, k := range keys {
		parts = append(parts,
			helpKeyStyle.Render(k.key)+" "+helpStyle.Render(k.desc))
	}
	return strings.Join(parts, helpStyle.Render("  •  "))
}

// AttachName returns the session name to attach to (if any) after the TUI
// exits. Returns empty when no attach was requested.
func (m Model) AttachName() string {
	return m.attachTarget.session
}

// AttachWindowIndex returns the window index selected for attachment, or -1
// if the user selected a session row.
func (m Model) AttachWindowIndex() int {
	return m.attachTarget.window
}

// AttachPaneIndex returns the pane index selected for attachment, or -1 if
// the user did not drill down to a pane row.
func (m Model) AttachPaneIndex() int {
	return m.attachTarget.pane
}

// AttachToSession switches to the target session, optionally focusing a
// specific window and pane first. Pass windowIdx == -1 to keep the active
// window; pass paneIdx == -1 to keep the active pane within that window.
//
// If already inside tmux, uses switch-client. Otherwise, uses attach-session.
func AttachToSession(name string, windowIdx, paneIdx int) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	// Focus the requested window/pane *before* attaching, since attach-session
	// replaces our process and we can't run anything afterwards.
	if windowIdx >= 0 {
		windowTarget := fmt.Sprintf("%s:%d", name, windowIdx)
		if err := exec.Command(tmuxPath, "select-window", "-t", windowTarget).Run(); err != nil {
			return fmt.Errorf("select-window %s: %w", windowTarget, err)
		}
		if paneIdx >= 0 {
			paneTarget := fmt.Sprintf("%s.%d", windowTarget, paneIdx)
			if err := exec.Command(tmuxPath, "select-pane", "-t", paneTarget).Run(); err != nil {
				return fmt.Errorf("select-pane %s: %w", paneTarget, err)
			}
		}
	}

	if os.Getenv("TMUX") != "" {
		return exec.Command(tmuxPath, "switch-client", "-t", name).Run()
	}
	return syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", name}, os.Environ())
}
