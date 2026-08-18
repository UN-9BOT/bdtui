package daemon

import (
	"os"
	"path/filepath"
)

const (
	stateDirName = "bdtui"
	socketName   = "bdtuid.sock"
	dbName       = "orchestrator.db"
)

// StateDir returns the daemon state directory, honoring XDG_STATE_HOME and
// falling back to ~/.local/state. The directory is not created here; the
// daemon and client create it on demand.
func StateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, stateDirName)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), stateDirName)
	}
	return filepath.Join(home, ".local", "state", stateDirName)
}

// DefaultSocketPath returns the well-known Unix domain socket path.
func DefaultSocketPath() string {
	return filepath.Join(StateDir(), socketName)
}

// DefaultDBPath returns the well-known orchestrator SQLite database path.
func DefaultDBPath() string {
	return filepath.Join(StateDir(), dbName)
}
