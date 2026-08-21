package daemon

import (
	"os"
	"path/filepath"
)

const (
	stateDirName = "bdtui"
	socketName   = "bdtuid.sock"
	dbName       = "orchestrator.db"
	// projectDirName is the per-project subdirectory under StateDir that
	// holds the per-project socket/db/pidfile/lock for the MVP daemon.
	// Multi-project dispatch within a single daemon (project_id ->
	// TaskStore registry) is the scope of bdtui-cvy.11; per-project
	// socket isolation is the prerequisite of that work and is
	// sufficient for the "Run from one workspace at a time" workflow.
	projectDirName = "projects"
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

// DefaultSocketPath returns the well-known global Unix domain socket path.
// This is the legacy single-tenant socket used for backwards compatibility
// with tests and tools that do not yet opt in to per-project routing.
// Production TUI launches go through SocketPathForProject.
func DefaultSocketPath() string {
	return filepath.Join(StateDir(), socketName)
}

// DefaultDBPath returns the well-known global orchestrator SQLite database
// path. Production TUI launches go through DBPathForProject.
func DefaultDBPath() string {
	return filepath.Join(StateDir(), dbName)
}

// SocketPathForProject returns the per-project Unix domain socket path.
// The project_id is the durable UUID assigned by the TUI; isolating the
// socket per project lets two repos have two daemons running side by
// side without colliding, and removes the "TUI for B silently connects
// to the daemon for A" risk flagged in review. Empty project_id falls
// back to the global legacy path.
func SocketPathForProject(projectID string) string {
	if projectID == "" {
		return DefaultSocketPath()
	}
	return filepath.Join(ProjectDir(projectID), socketName)
}

// DBPathForProject returns the per-project orchestrator SQLite database
// path. Isolation matches the per-project socket: a daemon only ever
// mutates the project that spawned it.
func DBPathForProject(projectID string) string {
	if projectID == "" {
		return DefaultDBPath()
	}
	return filepath.Join(ProjectDir(projectID), dbName)
}

// ProjectDir returns the per-project state directory. Layout:
//
//	$XDG_STATE_HOME/bdtui/projects/<project_id>/
//	  bdtuid.sock
//	  orchestrator.db
//	  bdtuid.pid
//	  bdtuid.lock
func ProjectDir(projectID string) string {
	return filepath.Join(StateDir(), projectDirName, projectID)
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
