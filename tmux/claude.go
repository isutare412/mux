package tmux

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	claudeSessionsTTL = 10 * time.Second
	claudeDir         = ".claude"
	sessionsDir       = "sessions"
	projectsDir       = "projects"
)

// TokenUsage holds aggregated token counts and estimated cost for a Claude session.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
	TotalCost    float64 // estimated USD
}

// claudeSessionFile represents the JSON structure of ~/.claude/sessions/{PID}.json.
type claudeSessionFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updatedAt"` // unix milliseconds
}

// jsonlMessage is a minimal representation of a JSONL line with usage data.
type jsonlMessage struct {
	Type    string `json:"type"`
	Message struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type cachedUsage struct {
	usage     *TokenUsage
	expiresAt time.Time
}

var (
	usageCache   = make(map[string]cachedUsage) // sessionID → cached usage
	usageCacheMu sync.Mutex
)

const claudeInfoTTL = 3 * time.Second

type cachedClaudeInfo struct {
	info      *ClaudeInfo
	expiresAt time.Time
}

var (
	claudeInfoCache   = make(map[string]cachedClaudeInfo) // sessionID → info
	claudeInfoCacheMu sync.Mutex
)

const configDirEnvTTL = 30 * time.Second

type cachedConfigDirEnv struct {
	value     string
	expiresAt time.Time
}

var (
	configDirEnvCache   = make(map[int]cachedConfigDirEnv) // pid → CLAUDE_CONFIG_DIR value
	configDirEnvCacheMu sync.Mutex
)

// configDirEnv returns the CLAUDE_CONFIG_DIR value for process pid, memoized with
// a short TTL because process env reads (ps eww) are expensive and loadClaudeInfo
// runs on every refresh tick.
func configDirEnv(pid int) string {
	configDirEnvCacheMu.Lock()
	if c, ok := configDirEnvCache[pid]; ok && time.Now().Before(c.expiresAt) {
		configDirEnvCacheMu.Unlock()
		return c.value
	}
	configDirEnvCacheMu.Unlock()

	v := configDirEnvReader(pid)

	configDirEnvCacheMu.Lock()
	configDirEnvCache[pid] = cachedConfigDirEnv{value: v, expiresAt: time.Now().Add(configDirEnvTTL)}
	configDirEnvCacheMu.Unlock()
	return v
}

// FindClaudeSession locates a Claude Code session for a given tmux pane PID and
// returns its session ID, working dir, and the config dir it was found in.
func FindClaudeSession(panePID int) (sessionID, cwd, configDir string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", err
	}
	sf, dir, err := resolveClaudeSession(home, panePID)
	if err != nil {
		return "", "", "", err
	}
	return sf.SessionID, sf.CWD, dir, nil
}

// LoadClaudeInfo resolves the Claude session for a tmux pane PID and returns its
// current state, recap, and last-activity time. Results are cached briefly by
// sessionID. Returns an error when the pane has no Claude child process.
func LoadClaudeInfo(panePID int) (*ClaudeInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sf, configDir, err := resolveClaudeSession(home, panePID)
	if err != nil {
		return nil, err
	}
	return buildClaudeInfo(configDir, sf), nil
}

// readSessionFile reads and decodes configDir/sessions/<pidStr>.json. The bool is
// false when the file is missing or malformed.
func readSessionFile(configDir, pidStr string) (claudeSessionFile, bool) {
	data, err := os.ReadFile(filepath.Join(configDir, sessionsDir, pidStr+".json"))
	if err != nil {
		return claudeSessionFile{}, false
	}
	var sf claudeSessionFile
	if json.Unmarshal(data, &sf) != nil {
		return claudeSessionFile{}, false
	}
	return sf, true
}

// resolveClaudeSession finds the Claude session file for a tmux pane PID and the
// config dir it lives in. It scans the pane shell's child PIDs in two passes:
// pass 1 checks the default ~/.claude (cheap stats, no env reads); pass 2 reads
// each child's CLAUDE_CONFIG_DIR and checks that dir. The two passes ensure
// default-config panes never trigger a process-env read.
func resolveClaudeSession(home string, panePID int) (claudeSessionFile, string, error) {
	out, err := runner.Output("pgrep", "-P", fmt.Sprintf("%d", panePID))
	if err != nil {
		return claudeSessionFile{}, "", fmt.Errorf("no child processes for pane %d", panePID)
	}
	pids := strings.Fields(string(out))

	defaultDir := filepath.Join(home, claudeDir)
	for _, pidStr := range pids {
		if sf, ok := readSessionFile(defaultDir, pidStr); ok {
			return sf, defaultDir, nil
		}
	}

	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		env := configDirEnv(pid)
		if env == "" {
			continue
		}
		dir := expandHome(env, home)
		if dir == defaultDir {
			continue
		}
		if sf, ok := readSessionFile(dir, pidStr); ok {
			return sf, dir, nil
		}
	}

	return claudeSessionFile{}, "", fmt.Errorf("no claude session found for pane %d", panePID)
}

// buildClaudeInfo assembles (and caches) a ClaudeInfo from a session file
// located in configDir.
func buildClaudeInfo(configDir string, sf claudeSessionFile) *ClaudeInfo {
	claudeInfoCacheMu.Lock()
	if c, ok := claudeInfoCache[sf.SessionID]; ok && time.Now().Before(c.expiresAt) {
		claudeInfoCacheMu.Unlock()
		return c.info
	}
	claudeInfoCacheMu.Unlock()

	jsonlPath := filepath.Join(configDir, projectsDir, encodePath(sf.CWD), sf.SessionID+".jsonl")
	awaiting, _ := transcriptAwaitingTool(jsonlPath)
	recap, _ := loadRecap(jsonlPath)

	info := &ClaudeInfo{
		State: deriveClaudeState(sf.Status, awaiting),
		Recap: recap,
		Since: time.UnixMilli(sf.UpdatedAt),
	}

	claudeInfoCacheMu.Lock()
	claudeInfoCache[sf.SessionID] = cachedClaudeInfo{info: info, expiresAt: time.Now().Add(claudeInfoTTL)}
	claudeInfoCacheMu.Unlock()
	return info
}

// LoadTokenUsage reads and aggregates token usage from a Claude session's JSONL
// log in configDir. Results are cached with a TTL to avoid re-reading large files.
func LoadTokenUsage(sessionID, cwd, configDir string) (*TokenUsage, error) {
	usageCacheMu.Lock()
	if cached, ok := usageCache[sessionID]; ok && time.Now().Before(cached.expiresAt) {
		usageCacheMu.Unlock()
		return cached.usage, nil
	}
	usageCacheMu.Unlock()

	jsonlPath := filepath.Join(configDir, projectsDir, encodePath(cwd), sessionID+".jsonl")

	usage, err := parseTokenUsage(jsonlPath)
	if err != nil {
		return nil, err
	}

	usageCacheMu.Lock()
	usageCache[sessionID] = cachedUsage{
		usage:     usage,
		expiresAt: time.Now().Add(claudeSessionsTTL),
	}
	usageCacheMu.Unlock()

	return usage, nil
}

func parseTokenUsage(path string) (*TokenUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	usage := &TokenUsage{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 1024*1024) // handle large lines

	for scanner.Scan() {
		line := scanner.Bytes()

		// Quick filter: skip lines without "usage"
		if !containsBytes(line, []byte(`"usage"`)) {
			continue
		}

		var msg jsonlMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Type != "assistant" {
			continue
		}

		u := msg.Message.Usage
		usage.InputTokens += u.InputTokens
		usage.OutputTokens += u.OutputTokens
		usage.CacheRead += u.CacheReadInputTokens
		usage.CacheWrite += u.CacheCreationInputTokens
	}

	usage.TotalCost = estimateCost(usage)
	return usage, nil
}

// estimateCost calculates an approximate USD cost from token counts.
// Uses Claude Opus 4.6 pricing as default.
func estimateCost(u *TokenUsage) float64 {
	const (
		inputPer1M      = 15.0
		outputPer1M     = 75.0
		cacheReadPer1M  = 1.5
		cacheWritePer1M = 18.75
	)
	cost := float64(u.InputTokens) / 1_000_000 * inputPer1M
	cost += float64(u.OutputTokens) / 1_000_000 * outputPer1M
	cost += float64(u.CacheRead) / 1_000_000 * cacheReadPer1M
	cost += float64(u.CacheWrite) / 1_000_000 * cacheWritePer1M
	return cost
}

// encodePath converts a filesystem path to the Claude projects directory encoding.
// "/Users/foo/bar" → "-Users-foo-bar"
func encodePath(path string) string {
	return strings.ReplaceAll(path, string(os.PathSeparator), "-")
}

// expandHome expands a leading ~ or $HOME in path to the home directory. Paths
// that are already absolute (or empty) are returned unchanged. CLAUDE_CONFIG_DIR
// is usually pre-expanded by the shell, but a literal ~ is handled defensively.
func expandHome(path, home string) string {
	switch {
	case path == "~", path == "$HOME":
		return home
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:])
	case strings.HasPrefix(path, "$HOME/"):
		return filepath.Join(home, path[len("$HOME/"):])
	default:
		return path
	}
}

const configDirEnvKey = "CLAUDE_CONFIG_DIR"

// configDirFromTokens returns the value of CLAUDE_CONFIG_DIR among a list of
// KEY=VALUE tokens (whitespace-split `ps` output or NUL-split /proc environ),
// or "" when the key is absent or has an empty value. The last match wins.
func configDirFromTokens(tokens []string) string {
	prefix := configDirEnvKey + "="
	val := ""
	for _, t := range tokens {
		if strings.HasPrefix(t, prefix) {
			val = strings.TrimPrefix(t, prefix)
		}
	}
	return val
}

// FormatTokens formats a token count into a short human-readable string.
func FormatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func containsBytes(haystack, needle []byte) bool {
	return strings.Contains(string(haystack), string(needle))
}

// deriveClaudeState maps a session status string and a pending-tool signal to a
// ClaudeState. When CC is "busy" it is actively running (auto-approved tools
// included), so that always reads as working. Only when it is NOT busy and a
// tool call is still pending do we treat it as waiting for permission.
func deriveClaudeState(status string, awaitingTool bool) ClaudeState {
	if status == "busy" {
		return ClaudeWorking
	}
	if awaitingTool {
		return ClaudeWaiting
	}
	return ClaudeIdle
}

// recapTailBytes bounds how much of the (possibly large) transcript tail we
// read when looking for the latest recap source / pending tool. ai-title is
// re-stamped every turn (always near the tail), but away_summary is written
// only at idle points, so we read a larger window to catch it in long sessions.
const recapTailBytes = 256 * 1024

// tailBytes returns up to the last n bytes of the file at path.
func tailBytes(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := int64(0)
	if info.Size() > n {
		start = info.Size() - n
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}

// transcriptAwaitingTool reports whether the transcript tail ends with an
// assistant tool_use that has no following user tool_result — i.e. a tool call
// is outstanding. Walking the tail in order, each assistant tool_use sets the
// flag and each user tool_result clears it, so the final value reflects the
// last unpaired tool_use.
func transcriptAwaitingTool(path string) (bool, error) {
	data, err := tailBytes(path, recapTailBytes)
	if err != nil {
		return false, err
	}
	awaiting := false
	for _, line := range bytes.Split(data, []byte("\n")) {
		var m struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		switch m.Type {
		case "assistant":
			if bytes.Contains(m.Message.Content, []byte(`"tool_use"`)) {
				awaiting = true
			}
		case "user":
			if bytes.Contains(m.Message.Content, []byte(`"tool_result"`)) {
				awaiting = false
			}
		}
	}
	return awaiting, nil
}

// goalPrefixRe matches a leading "Goal", "Goal was", "Goal is", "Goal to",
// "Goal was to", etc., with an optional trailing colon, so recaps read
// "colorizing the picker" instead of "Goal was colorizing the picker". The
// \b prevents stripping words like "Goals".
var goalPrefixRe = regexp.MustCompile(`(?i)^goal\b(\s+(was|is))?(\s+to)?\s*:?\s*`)

// firstSentence returns s up to and including the first sentence-terminating
// '.', '!' or '?' that is at end-of-string or followed by whitespace. Sentence
// terminators are ASCII, so a byte scan is sufficient.
func firstSentence(s string) string {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.', '!', '?':
			if i+1 >= len(s) || s[i+1] == ' ' || s[i+1] == '\n' || s[i+1] == '\t' {
				return s[:i+1]
			}
		}
	}
	return s
}

// cleanRecapText normalizes a raw recap source (away_summary, ai-title, or last
// assistant text) into a single tidy line for the list view.
func cleanRecapText(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(strings.ToLower(s), "(disable recaps in /config)"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	s = firstSentence(s)
	s = goalPrefixRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.TrimRight(s, ".!?")
	return strings.TrimSpace(s)
}

// lastAssistantText returns the joined text content of the most recent
// assistant message that has any text parts, scanning from the end. Assistant
// turns that contain only tool_use (no prose) are skipped. Returns "" if none.
func lastAssistantText(lines [][]byte) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"assistant"`)) {
			continue
		}
		var m struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &m) != nil || m.Type != "assistant" {
			continue
		}
		var parts []string
		for _, c := range m.Message.Content {
			if c.Type == "text" && c.Text != "" {
				parts = append(parts, c.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	return ""
}

// loadRecap returns a cleaned one-line recap for the transcript at path,
// selecting the first available source in priority order:
//  1. away_summary  (Claude Code's "Recap" — richest, subject-accurate)
//  2. ai-title      (session title; re-stamped each turn, always near the tail)
//  3. last assistant text (covers brand-new sessions before 1/2 exist)
//  4. ""            (none available)
func loadRecap(path string) (string, error) {
	data, err := tailBytes(path, recapTailBytes)
	if err != nil {
		return "", err
	}
	lines := bytes.Split(data, []byte("\n"))

	away, ai := "", ""
	for _, line := range lines {
		if bytes.Contains(line, []byte(`"away_summary"`)) {
			var m struct {
				Type    string `json:"type"`
				Subtype string `json:"subtype"`
				Content string `json:"content"`
			}
			if json.Unmarshal(line, &m) == nil && m.Type == "system" && m.Subtype == "away_summary" {
				away = m.Content
			}
			continue
		}
		if bytes.Contains(line, []byte(`"ai-title"`)) {
			var m struct {
				Type    string `json:"type"`
				AITitle string `json:"aiTitle"`
			}
			if json.Unmarshal(line, &m) == nil && m.Type == "ai-title" {
				ai = m.AITitle
			}
		}
	}

	if away != "" {
		return cleanRecapText(away), nil
	}
	if ai != "" {
		return cleanRecapText(ai), nil
	}
	if t := lastAssistantText(lines); t != "" {
		return cleanRecapText(t), nil
	}
	return "", nil
}
