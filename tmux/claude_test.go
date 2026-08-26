package tmux

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTranscriptPathDirectHit(t *testing.T) {
	configDir := t.TempDir()
	cwd := "/Users/me/repo/main"
	proj := filepath.Join(configDir, "projects", encodePath(cwd))
	if err := os.MkdirAll(proj, 0755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(proj, "s.jsonl")
	if err := os.WriteFile(want, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := resolveTranscriptPath(configDir, cwd, "s"); got != want {
		t.Errorf("resolveTranscriptPath = %q, want direct hit %q", got, want)
	}
}

func TestResolveTranscriptPathWorktreeFallback(t *testing.T) {
	configDir := t.TempDir()
	// Claude Code files a worktree session's transcript under the MAIN repo's
	// project dir, not the worktree cwd's encoded dir.
	mainProj := filepath.Join(configDir, "projects", "-Users-me-repo-main")
	if err := os.MkdirAll(mainProj, 0755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(mainProj, "abc-123.jsonl")
	if err := os.WriteFile(want, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	// The worktree cwd encodes to a different, nonexistent project dir.
	got := resolveTranscriptPath(configDir, "/Users/me/repo/wt-feature", "abc-123")
	if got != want {
		t.Errorf("resolveTranscriptPath = %q, want worktree fallback %q", got, want)
	}
}

func TestResolveTranscriptPathMissingReturnsDirect(t *testing.T) {
	configDir := t.TempDir()
	cwd := "/Users/me/repo/main"
	want := filepath.Join(configDir, "projects", encodePath(cwd), "none.jsonl")
	// No file exists anywhere; should return the direct encoded path so
	// downstream open/stat error handling is unchanged.
	if got := resolveTranscriptPath(configDir, cwd, "none"); got != want {
		t.Errorf("resolveTranscriptPath = %q, want direct path %q", got, want)
	}
}

func TestEncodePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/Users/foo/bar", "-Users-foo-bar"},
		{"/home/user/project", "-home-user-project"},
		{"", ""},
	}
	for _, tt := range tests {
		got := encodePath(tt.input)
		if got != tt.want {
			t.Errorf("encodePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{500, "500"},
		{1500, "1.5k"},
		{45200, "45.2k"},
		{1500000, "1.5M"},
		{0, "0"},
	}
	for _, tt := range tests {
		got := FormatTokens(tt.input)
		if got != tt.want {
			t.Errorf("FormatTokens(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEstimateCost(t *testing.T) {
	u := &TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 100_000,
		CacheRead:    500_000,
		CacheWrite:   200_000,
	}
	cost := estimateCost(u)
	// Input: 1M * $15/1M = $15
	// Output: 0.1M * $75/1M = $7.5
	// CacheRead: 0.5M * $1.5/1M = $0.75
	// CacheWrite: 0.2M * $18.75/1M = $3.75
	// Total = $27.0
	expected := 27.0
	if cost < expected-0.01 || cost > expected+0.01 {
		t.Errorf("estimateCost() = %f, want ~%f", cost, expected)
	}
}

func TestParseTokenUsage(t *testing.T) {
	// Create a temp JSONL file with sample data
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")

	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":200,"cache_creation_input_tokens":300}}}`,
		`{"type":"file-history-snapshot","snapshot":{}}`,
		`{"type":"assistant","message":{"model":"claude-opus-4-6","role":"assistant","usage":{"input_tokens":150,"output_tokens":75,"cache_read_input_tokens":100,"cache_creation_input_tokens":0}}}`,
	}

	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	usage, err := parseTokenUsage(path)
	if err != nil {
		t.Fatal(err)
	}

	if usage.InputTokens != 250 {
		t.Errorf("InputTokens = %d, want 250", usage.InputTokens)
	}
	if usage.OutputTokens != 125 {
		t.Errorf("OutputTokens = %d, want 125", usage.OutputTokens)
	}
	if usage.CacheRead != 300 {
		t.Errorf("CacheRead = %d, want 300", usage.CacheRead)
	}
	if usage.CacheWrite != 300 {
		t.Errorf("CacheWrite = %d, want 300", usage.CacheWrite)
	}
	if usage.TotalCost <= 0 {
		t.Error("TotalCost should be positive")
	}
}

func TestParseTokenUsageMissingFile(t *testing.T) {
	_, err := parseTokenUsage("/nonexistent/path.jsonl")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestDeriveClaudeState(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		awaiting bool
		want     ClaudeState
	}{
		{"busy is working", "busy", false, ClaudeWorking},
		{"busy stays working even with pending tool", "busy", true, ClaudeWorking},
		{"not busy with pending tool waits", "idle", true, ClaudeWaiting},
		{"not busy no pending tool is idle", "idle", false, ClaudeIdle},
		{"empty status no pending is idle", "", false, ClaudeIdle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveClaudeState(tt.status, tt.awaiting)
			if got != tt.want {
				t.Errorf("deriveClaudeState(%q, %v) = %d, want %d", tt.status, tt.awaiting, got, tt.want)
			}
		})
	}
}

func TestLoadRecap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"ai-title","aiTitle":"First title","sessionId":"s"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text"}]}}`,
		`{"type":"ai-title","aiTitle":"Latest title","sessionId":"s"}`,
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadRecap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Latest title" {
		t.Errorf("loadRecap = %q, want %q", got, "Latest title")
	}
}

func TestLoadRecapNoTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadRecap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("loadRecap = %q, want empty", got)
	}
}

func TestLoadRecapMissingFile(t *testing.T) {
	if _, err := loadRecap("/nonexistent/x.jsonl"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCleanRecapText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"strip disable-recaps suffix and Goal prefix",
			"Goal was colorizing the tmux picker: bold names. Both tasks done. (disable recaps in /config)",
			"colorizing the tmux picker: bold names"},
		{"first sentence only",
			"Built a flash-style jump mode. Press s then a letter.",
			"Built a flash-style jump mode"},
		{"Goal colon prefix and trailing dotted path",
			"Goal: make mux detect ~/.claude and ~/.claude-enterprise.",
			"make mux detect ~/.claude and ~/.claude-enterprise"},
		{"Goal is prefix",
			"Goal is preparing for the interview.",
			"preparing for the interview"},
		{"strip backticks, Goal was to",
			"Goal was to extend the `x` key to kill panes.",
			"extend the x key to kill panes"},
		{"plain ai-title untouched",
			"Cancel merge and rebase to main",
			"Cancel merge and rebase to main"},
		{"does not strip Goals (word boundary)",
			"Goals matter here.",
			"Goals matter here"},
		{"empty stays empty",
			"",
			""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cleanRecapText(c.in); got != c.want {
				t.Errorf("cleanRecapText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// custom-title (user-set via /rename) is the top priority and must win over
// away_summary, last-assistant text, and ai-title.
func TestLoadRecapPrefersCustomTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	lines := []string{
		`{"type":"custom-title","customTitle":"My renamed session"}`,
		`{"type":"ai-title","aiTitle":"Rebase branch into main","sessionId":"s"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Some prose."}]}}`,
		`{"type":"system","subtype":"away_summary","content":"Goal was colorizing the picker. Done."}`,
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadRecap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "My renamed session" {
		t.Errorf("loadRecap = %q, want custom-title to win over all other sources", got)
	}
}

// With no custom-title, away_summary must win over a frozen/off-topic ai-title.
func TestLoadRecapAwayBeatsAiTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	lines := []string{
		`{"type":"ai-title","aiTitle":"Rebase branch into main","sessionId":"s"}`,
		`{"type":"system","subtype":"away_summary","content":"Goal was colorizing the tmux picker: bold names. Done. (disable recaps in /config)"}`,
		`{"type":"ai-title","aiTitle":"Rebase branch into main","sessionId":"s"}`,
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadRecap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "colorizing the tmux picker: bold names" {
		t.Errorf("loadRecap = %q, want away_summary to win over ai-title", got)
	}
}

// ai-title is the last resort: last-assistant text outranks it.
func TestLoadRecapAssistantBeatsAiTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	lines := []string{
		`{"type":"ai-title","aiTitle":"Fix pnpm build failure","sessionId":"s"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Wiring up the parser."}]}}`,
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadRecap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Wiring up the parser" {
		t.Errorf("loadRecap = %q, want last-assistant text to win over ai-title", got)
	}
}

// ai-title is still used when it is the only source available.
func TestLoadRecapAiTitleLastResort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"ai-title","aiTitle":"Fix pnpm build failure","sessionId":"s"}`,
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadRecap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Fix pnpm build failure" {
		t.Errorf("loadRecap = %q, want ai-title as last resort", got)
	}
}

func TestLoadRecapFallsBackToLastAssistant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"start"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I am refactoring the loader. More detail follows."}]}}`,
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadRecap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "I am refactoring the loader" {
		t.Errorf("loadRecap = %q, want first sentence of last assistant text", got)
	}
}

// away_summary is the second-priority source: with no ai-title present it must
// win over a last-assistant text message.
func TestLoadRecapAwayBeatsAssistant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	lines := []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Some assistant prose."}]}}`,
		`{"type":"system","subtype":"away_summary","content":"Goal was colorizing the picker. Done."}`,
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadRecap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "colorizing the picker" {
		t.Errorf("loadRecap = %q, want away_summary to win over assistant text", got)
	}
}

// The cleanup pipeline must apply to the last-assistant fallback source too
// (Goal prefix + backticks stripped, first sentence only).
func TestLoadRecapCleansLastAssistant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"start"}}`,
		"{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Goal was to wire up the `parser` module. Next step follows.\"}]}}",
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadRecap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "wire up the parser module" {
		t.Errorf("loadRecap = %q, want cleaned last-assistant text", got)
	}
}

// joinLines joins JSONL lines with trailing newlines.
func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

func TestClaudeSessionFileDecode(t *testing.T) {
	raw := []byte(`{"pid":81767,"sessionId":"abc","cwd":"/tmp/x","status":"busy","updatedAt":1781083530418}`)
	var sf claudeSessionFile
	if err := json.Unmarshal(raw, &sf); err != nil {
		t.Fatal(err)
	}
	if sf.Status != "busy" {
		t.Errorf("Status = %q, want busy", sf.Status)
	}
	if sf.UpdatedAt != 1781083530418 {
		t.Errorf("UpdatedAt = %d, want 1781083530418", sf.UpdatedAt)
	}
	if sf.SessionID != "abc" || sf.CWD != "/tmp/x" {
		t.Errorf("SessionID/CWD = %q/%q", sf.SessionID, sf.CWD)
	}
}

func TestExpandHome(t *testing.T) {
	home := "/Users/alice"
	tests := []struct {
		in   string
		want string
	}{
		{"~", "/Users/alice"},
		{"~/.claude-enterprise", "/Users/alice/.claude-enterprise"},
		{"$HOME", "/Users/alice"},
		{"$HOME/.claude-enterprise", "/Users/alice/.claude-enterprise"},
		{"/opt/claude-work", "/opt/claude-work"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := expandHome(tt.in, home); got != tt.want {
			t.Errorf("expandHome(%q, %q) = %q, want %q", tt.in, home, got, tt.want)
		}
	}
}

func TestConfigDirFromTokens(t *testing.T) {
	tests := []struct {
		name   string
		tokens []string
		want   string
	}{
		{
			name:   "ps-style fields",
			tokens: []string{"claude", "--foo", "CLAUDE_CONFIG_DIR=/Users/x/.claude-enterprise", "TERM=xterm"},
			want:   "/Users/x/.claude-enterprise",
		},
		{
			name:   "environ-style tokens",
			tokens: []string{"PATH=/usr/bin", "CLAUDE_CONFIG_DIR=/opt/claude-work", "HOME=/Users/x"},
			want:   "/opt/claude-work",
		},
		{
			name:   "absent",
			tokens: []string{"PATH=/usr/bin", "HOME=/Users/x"},
			want:   "",
		},
		{
			name:   "empty value",
			tokens: []string{"CLAUDE_CONFIG_DIR="},
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := configDirFromTokens(tt.tokens); got != tt.want {
				t.Errorf("configDirFromTokens = %q, want %q", got, tt.want)
			}
		})
	}
}

// stubConfigDirEnv replaces the platform env reader with fn and clears the env
// cache, restoring both on test cleanup. The returned counter records how many
// times fn was invoked (i.e. cache misses).
func stubConfigDirEnv(t *testing.T, fn func(pid int) string) *int {
	t.Helper()
	calls := 0
	old := configDirEnvReader
	configDirEnvReader = func(pid int) string {
		calls++
		return fn(pid)
	}
	configDirEnvCacheMu.Lock()
	configDirEnvCache = make(map[int]cachedConfigDirEnv)
	configDirEnvCacheMu.Unlock()
	t.Cleanup(func() { configDirEnvReader = old })
	return &calls
}

func TestConfigDirEnvCaches(t *testing.T) {
	calls := stubConfigDirEnv(t, func(pid int) string {
		return "/opt/claude-work"
	})
	if got := configDirEnv(9001); got != "/opt/claude-work" {
		t.Fatalf("configDirEnv = %q, want /opt/claude-work", got)
	}
	if got := configDirEnv(9001); got != "/opt/claude-work" {
		t.Fatalf("second configDirEnv = %q, want /opt/claude-work", got)
	}
	if *calls != 1 {
		t.Errorf("reader called %d times, want 1 (cached)", *calls)
	}
}

func TestTranscriptAwaitingTool(t *testing.T) {
	dir := t.TempDir()

	write := func(name string, lines []string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(joinLines(lines)), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	pending := write("pending.jsonl", []string{
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"go"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}]}}`,
	})
	resolved := write("resolved.jsonl", []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result"}]}}`,
	})
	idle := write("idle.jsonl", []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`,
	})

	cases := []struct {
		path string
		want bool
	}{
		{pending, true},
		{resolved, false},
		{idle, false},
	}
	for _, c := range cases {
		got, err := transcriptAwaitingTool(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("transcriptAwaitingTool(%s) = %v, want %v", filepath.Base(c.path), got, c.want)
		}
	}
}

func TestLastAssistantText(t *testing.T) {
	lines := [][]byte{
		[]byte(`{"type":"user","message":{"role":"user","content":"hi"}}`),
		[]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"First reply."}]}}`),
		[]byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}]}}`),
	}
	// Most-recent assistant turn is tool-only (no text); should fall back to the
	// previous assistant turn's text.
	if got := lastAssistantText(lines); got != "First reply." {
		t.Errorf("lastAssistantText = %q, want %q", got, "First reply.")
	}

	none := [][]byte{[]byte(`{"type":"user","message":{"role":"user","content":"hi"}}`)}
	if got := lastAssistantText(none); got != "" {
		t.Errorf("lastAssistantText(no assistant) = %q, want empty", got)
	}
}

// writeSessionFile creates configDir/sessions/<pid>.json with the given session.
func writeSessionFile(t *testing.T, configDir string, pid int, sf claudeSessionFile) {
	t.Helper()
	dir := filepath.Join(configDir, sessionsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.json", pid)), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFindClaudeSessionReturnsConfigDir(t *testing.T) {
	// Point HOME at a temp dir so os.UserHomeDir() inside FindClaudeSession
	// resolves there.
	home := t.TempDir()
	t.Setenv("HOME", home)

	entDir := filepath.Join(home, ".claude-enterprise")
	writeSessionFile(t, entDir, 200, claudeSessionFile{
		PID: 200, SessionID: "sid-ent", CWD: "/tmp/proj", Status: "busy",
	})
	stubConfigDirEnv(t, func(pid int) string {
		if pid == 200 {
			return "~/.claude-enterprise"
		}
		return ""
	})
	withMock(t, func(m *mockRunner) {
		m.OnOutput([]byte("200\n"), nil, "pgrep", "-P", "100")

		sessionID, cwd, configDir, err := FindClaudeSession(100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sessionID != "sid-ent" {
			t.Errorf("sessionID = %q, want sid-ent", sessionID)
		}
		if cwd != "/tmp/proj" {
			t.Errorf("cwd = %q, want /tmp/proj", cwd)
		}
		if configDir != entDir {
			t.Errorf("configDir = %q, want %q", configDir, entDir)
		}
	})
}

func TestLoadTokenUsageUsesConfigDir(t *testing.T) {
	configDir := t.TempDir()
	cwd := "/tmp/proj"
	sessionID := "sid-1"
	jsonlDir := filepath.Join(configDir, projectsDir, encodePath(cwd))
	if err := os.MkdirAll(jsonlDir, 0755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","message":{"role":"assistant","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(filepath.Join(jsonlDir, sessionID+".jsonl"), []byte(line), 0644); err != nil {
		t.Fatal(err)
	}

	usage, err := LoadTokenUsage(sessionID, cwd, configDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage.InputTokens != 100 || usage.OutputTokens != 50 {
		t.Errorf("usage = %+v, want input 100 output 50", usage)
	}
}

func TestResolveClaudeSession(t *testing.T) {
	t.Run("default dir, no env read", func(t *testing.T) {
		home := t.TempDir()
		writeSessionFile(t, filepath.Join(home, claudeDir), 200, claudeSessionFile{
			PID: 200, SessionID: "sid-default", CWD: "/tmp/proj", Status: "busy",
		})
		calls := stubConfigDirEnv(t, func(pid int) string { return "" })
		withMock(t, func(m *mockRunner) {
			m.OnOutput([]byte("199\n200\n"), nil, "pgrep", "-P", "100")

			sf, dir, err := resolveClaudeSession(home, 100)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sf.SessionID != "sid-default" {
				t.Errorf("SessionID = %q, want sid-default", sf.SessionID)
			}
			if want := filepath.Join(home, claudeDir); dir != want {
				t.Errorf("dir = %q, want %q", dir, want)
			}
			if *calls != 0 {
				t.Errorf("env reader called %d times, want 0 (fast path)", *calls)
			}
		})
	})

	t.Run("enterprise dir via env", func(t *testing.T) {
		home := t.TempDir()
		entDir := filepath.Join(home, ".claude-enterprise")
		writeSessionFile(t, entDir, 200, claudeSessionFile{
			PID: 200, SessionID: "sid-ent", CWD: "/tmp/proj", Status: "busy",
		})
		stubConfigDirEnv(t, func(pid int) string {
			if pid == 200 {
				return "~/.claude-enterprise"
			}
			return ""
		})
		withMock(t, func(m *mockRunner) {
			m.OnOutput([]byte("199\n200\n"), nil, "pgrep", "-P", "100")

			sf, dir, err := resolveClaudeSession(home, 100)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sf.SessionID != "sid-ent" {
				t.Errorf("SessionID = %q, want sid-ent", sf.SessionID)
			}
			if dir != entDir {
				t.Errorf("dir = %q, want %q", dir, entDir)
			}
		})
	})

	t.Run("no session found", func(t *testing.T) {
		home := t.TempDir()
		stubConfigDirEnv(t, func(pid int) string { return "" })
		withMock(t, func(m *mockRunner) {
			m.OnOutput([]byte("199\n200\n"), nil, "pgrep", "-P", "100")

			if _, _, err := resolveClaudeSession(home, 100); err == nil {
				t.Error("expected error when no session file exists")
			}
		})
	})

	t.Run("no child processes", func(t *testing.T) {
		home := t.TempDir()
		withMock(t, func(m *mockRunner) {
			m.OnOutput(nil, fmt.Errorf("no children"), "pgrep", "-P", "100")

			if _, _, err := resolveClaudeSession(home, 100); err == nil {
				t.Error("expected error when pgrep fails")
			}
		})
	})
}

// A recap with no ASCII sentence terminator — the norm for Korean summaries —
// survives firstSentence whole, newlines and all. The list view draws one row
// per item and measures width with ansi.StringWidth, which scores control
// characters as zero cells while the terminal acts on them: a tab advances to
// the next tab stop, a carriage return jumps to column 0, a newline opens a
// row. Any survivor desyncs mux's frame from the screen, so cleanRecapText
// must fold them all into single spaces.
func TestCleanRecapTextCollapsesControlCharacters(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"newline becomes a space",
			"README를 한글로 다시 쓰는 중이고\n요청하신 수정도 반영했습니다",
			"README를 한글로 다시 쓰는 중이고 요청하신 수정도 반영했습니다"},
		{"tab becomes a space",
			"작업\t내용\t정리",
			"작업 내용 정리"},
		{"carriage return becomes a space",
			"앞부분\r뒷부분",
			"앞부분 뒷부분"},
		{"runs of whitespace collapse to one space",
			"첫 줄\n\n  \t둘째 줄",
			"첫 줄 둘째 줄"},
		{"interior single spaces are preserved",
			"iptables나 ingress 설정을 확인",
			"iptables나 ingress 설정을 확인"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cleanRecapText(c.in); got != c.want {
				t.Errorf("cleanRecapText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
