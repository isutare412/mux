//go:build darwin

package tmux

import (
	"fmt"
	"strings"
)

// readProcessConfigDirEnv returns the CLAUDE_CONFIG_DIR value in process pid's
// environment, read via `ps eww`. Returns "" when unset or unreadable. Works for
// processes owned by the current user.
func readProcessConfigDirEnv(pid int) string {
	out, err := runner.Output("ps", "eww", "-p", fmt.Sprintf("%d", pid), "-o", "command=")
	if err != nil {
		return ""
	}
	return configDirFromTokens(strings.Fields(string(out)))
}

// configDirEnvReader is the platform process-env reader, replaceable in tests.
var configDirEnvReader = readProcessConfigDirEnv
