//go:build linux

package tmux

import (
	"fmt"
	"os"
	"strings"
)

// readProcessConfigDirEnv returns the CLAUDE_CONFIG_DIR value in process pid's
// environment, read from /proc/<pid>/environ. Returns "" when unset or unreadable.
func readProcessConfigDirEnv(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return ""
	}
	return configDirFromTokens(strings.Split(string(data), "\x00"))
}

// configDirEnvReader is the platform process-env reader, replaceable in tests.
var configDirEnvReader = readProcessConfigDirEnv
