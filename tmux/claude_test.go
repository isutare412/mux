package tmux

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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
