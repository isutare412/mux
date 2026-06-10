package tmux

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// FindClaudeSession locates a Claude Code session file for a given tmux pane PID.
// It scans child processes to find the Claude PID, then reads its session file.
func FindClaudeSession(panePID int) (sessionID string, cwd string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}

	// Get child PIDs of the pane shell
	out, err := runner.Output("pgrep", "-P", fmt.Sprintf("%d", panePID))
	if err != nil {
		return "", "", fmt.Errorf("no child processes for pane %d", panePID)
	}

	sessDir := filepath.Join(home, claudeDir, sessionsDir)

	for _, pidStr := range strings.Fields(string(out)) {
		path := filepath.Join(sessDir, pidStr+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var sf claudeSessionFile
		if err := json.Unmarshal(data, &sf); err != nil {
			continue
		}
		return sf.SessionID, sf.CWD, nil
	}

	return "", "", fmt.Errorf("no claude session found for pane %d", panePID)
}

// LoadClaudeInfo resolves the Claude session for a tmux pane PID and returns its
// current state, recap, and last-activity time. Results are cached briefly by
// sessionID. Returns an error when the pane has no Claude child process.
func LoadClaudeInfo(panePID int) (*ClaudeInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	out, err := runner.Output("pgrep", "-P", fmt.Sprintf("%d", panePID))
	if err != nil {
		return nil, fmt.Errorf("no child processes for pane %d", panePID)
	}

	sessDir := filepath.Join(home, claudeDir, sessionsDir)
	for _, pidStr := range strings.Fields(string(out)) {
		data, err := os.ReadFile(filepath.Join(sessDir, pidStr+".json"))
		if err != nil {
			continue
		}
		var sf claudeSessionFile
		if err := json.Unmarshal(data, &sf); err != nil {
			continue
		}
		return buildClaudeInfo(home, sf), nil
	}
	return nil, fmt.Errorf("no claude session found for pane %d", panePID)
}

// buildClaudeInfo assembles (and caches) a ClaudeInfo from a session file.
func buildClaudeInfo(home string, sf claudeSessionFile) *ClaudeInfo {
	claudeInfoCacheMu.Lock()
	if c, ok := claudeInfoCache[sf.SessionID]; ok && time.Now().Before(c.expiresAt) {
		claudeInfoCacheMu.Unlock()
		return c.info
	}
	claudeInfoCacheMu.Unlock()

	jsonlPath := filepath.Join(home, claudeDir, projectsDir, encodePath(sf.CWD), sf.SessionID+".jsonl")
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

// LoadTokenUsage reads and aggregates token usage from a Claude session's JSONL log.
// Results are cached with a TTL to avoid re-reading large files on every tick.
func LoadTokenUsage(sessionID, cwd string) (*TokenUsage, error) {
	usageCacheMu.Lock()
	if cached, ok := usageCache[sessionID]; ok && time.Now().Before(cached.expiresAt) {
		usageCacheMu.Unlock()
		return cached.usage, nil
	}
	usageCacheMu.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	encoded := encodePath(cwd)
	jsonlPath := filepath.Join(home, claudeDir, projectsDir, encoded, sessionID+".jsonl")

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
// read when looking for the latest ai-title / pending tool. ai-title lines are
// written frequently, so the most recent one is virtually always in the tail.
const recapTailBytes = 64 * 1024

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

// loadRecap returns the most recent ai-title in the transcript at path, or ""
// if none is present in the scanned tail.
func loadRecap(path string) (string, error) {
	data, err := tailBytes(path, recapTailBytes)
	if err != nil {
		return "", err
	}
	recap := ""
	for _, line := range bytes.Split(data, []byte("\n")) {
		if !bytes.Contains(line, []byte(`"ai-title"`)) {
			continue
		}
		var m struct {
			Type    string `json:"type"`
			AITitle string `json:"aiTitle"`
		}
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		if m.Type == "ai-title" {
			recap = m.AITitle
		}
	}
	return recap, nil
}
