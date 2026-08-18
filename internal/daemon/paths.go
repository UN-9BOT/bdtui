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

// EnsureStateDirs creates the parent directories of every file path it is
// given (socket, db, pidfile, lockfile). It is idempotent and safe to call
// before any daemon-owned file is created.
func EnsureStateDirs(paths ...string) error {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
	}
	return nil
}
