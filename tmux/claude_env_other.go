//go:build !darwin && !linux

package tmux

// readProcessConfigDirEnv has no implementation on this platform; only the
// default ~/.claude config dir is detected.
func readProcessConfigDirEnv(pid int) string {
	return ""
}

// configDirEnvReader is the platform process-env reader, replaceable in tests.
var configDirEnvReader = readProcessConfigDirEnv
