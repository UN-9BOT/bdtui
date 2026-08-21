package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bdtui/internal/daemon"
	"bdtui/internal/daemon/daemonpb"
	"bdtui/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
)

// defaultGlobalWorkflowsRoot is the fallback layout root the TUI passes to
// workflow.Loader for global workflow/role definitions. Per the Loader
// contract, the loader appends "/workflows/<name>.yaml" internally, so this
// must be the layout root, not the workflows directory itself.
const defaultGlobalWorkflowsRoot = "/usr/local/share/bdtui"

// gitProjectIDFilename is the file inside the repo's git directory that
// stores the durable project_id. Placing it under `<git-dir>/` rather than
// inside the tracked worktree guarantees the id is never visible in
// `git status` / `git diff` / `git ls-files` (git treats .git/ as its own
// metadata and never lists it), and survives workspace moves because the
// .git/ directory moves with the repo. A fresh clone gets a fresh id,
// which is the correct semantics for "different project".
const gitProjectIDFilename = ".bdtui-project-id"

// projectWorkflowsRoot is the project layout root for workflow.Loader. Per
// the Loader contract, roots are layout directories and the loader appends
// "/workflows/<name>.yaml" internally — passing <beads-dir>/workflows here
// would lead to a double "workflows" path and miss every file.
func (m model) projectWorkflowsRoot() string {
	if strings.TrimSpace(m.BeadsDir) == "" {
		return ""
	}
	return m.BeadsDir
}

// resolveWorkflowsRoots returns (global, project) roots the TUI should pass
// to workflow.Loader. An empty root means "not configured"; the loader
// silently skips missing roots.
func (m model) resolveWorkflowsRoots() (string, string) {
	return defaultGlobalWorkflowsRoot, m.projectWorkflowsRoot()
}

// loadWorkflowOptions lists every workflow visible to the TUI, sorted by name
// with project entries preferred over globals on collision.
func (m model) loadWorkflowOptions() ([]WorkflowOption, error) {
	global, project := m.resolveWorkflowsRoots()
	loader := workflow.Loader{Global: global, Project: project}
	entries, err := loader.List(context.Background())
	if err != nil {
		return nil, err
	}
	options := make([]WorkflowOption, 0, len(entries))
	for _, e := range entries {
		options = append(options, WorkflowOption{Name: e.Name, Origin: e.Origin})
	}
	return options, nil
}

// launchRunCmd resolves the named workflow into a snapshot and sends
// CreateRun to the daemon. The daemon claims the task in the
// TaskStore (Beads) atomically inside CreateRun, so the TUI does not
// need to perform a separate `bd update` — splitting the operation
// into two non-atomic steps would leave a queued Run with no Beads
// claim, and a follow-up retry would then hit ErrActiveRunExists.
//
// The TaskStore encodes only the high-level task lifecycle (todo /
// in_progress / done / blocked). Queued / running / current-step state
// remains in the orchestrator's SQLite store and is surfaced via the
// events / runs RPC, not by mutating Beads.
func (m model) launchRunCmd(taskID, workflowName string) tea.Cmd {
	workflowName = strings.TrimSpace(workflowName)
	taskID = strings.TrimSpace(taskID)
	if workflowName == "" || taskID == "" {
		return opCmd("workflow name and task id are required", func() error {
			return fmt.Errorf("workflow: empty workflow name or task id")
		})
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()

		client, err := m.ensureDaemon(ctx)
		if err != nil {
			return opMsg{err: fmt.Errorf("daemon: %w", err)}
		}
		defer client.Close()

		snapshot, err := m.snapshotWorkflow(ctx, workflowName)
		if err != nil {
			return opMsg{err: err}
		}

		projectID, err := m.projectIDForBeadsDir()
		if err != nil {
			return opMsg{err: fmt.Errorf("project id: %w", err)}
		}
		if projectID == "" {
			return opMsg{err: fmt.Errorf("project id unavailable")}
		}
		run, err := client.CreateRun(ctx, &daemonpb.CreateRunRequest{
			ProjectId:           projectID,
			TaskId:              taskID,
			WorkflowSnapshotRef: snapshot.Ref,
			WorkflowSnapshot:    snapshot.JSON,
		})
		if err != nil {
			return opMsg{err: fmt.Errorf("create run: %w", err)}
		}
		return opMsg{info: fmt.Sprintf("run %s started", run.Id)}
	}
}

// projectIDForBeadsDir returns the durable project_id stored at
// `<git-dir>/.bdtui-project-id`, generating + persisting a fresh UUID hex
// (no dashes) on first use. The id is opaque, machine-independent, and
// survives moves of the workspace directory (since .git/ moves with the
// repo and the file lives inside it, not in the tracked worktree).
//
// Concurrency: the file lives inside .git/ (which is git-local metadata
// and never tracked). We publish it atomically via a sibling temp file +
// os.Link — same pattern as before. At most one concurrent caller wins
// the link (returns its own id); all losers observe EEXIST and re-read
// the winner's fully-flushed bytes via a tiny backoff loop. This is
// strictly stronger than `git config --local` because that approach lets
// concurrent writers race sequential reads during the same window.
//
// On any error we surface it to the caller rather than silently round-trip
// with a fresh id — sending a daemon call with a non-durable id would defeat
// the active-run uniqueness guarantee.
func (m model) projectIDForBeadsDir() (string, error) {
	repoDir := strings.TrimSpace(m.RepoDir)
	if repoDir == "" {
		return "", fmt.Errorf("no repo-dir configured (project id needs a git workspace)")
	}

	gitDir, err := resolveGitDir(repoDir)
	if err != nil {
		return "", fmt.Errorf("project id requires a git workspace: %w", err)
	}
	path := filepath.Join(gitDir, gitProjectIDFilename)

	// Probe: read once. We never treat empty/invalid content as a hard
	// error here, because a concurrent first-time publisher may have
	// created the file but not yet flushed its bytes. Falling through to
	// the publish path keeps the file self-healing and lets concurrent
	// callers re-discover the winner via EEXIST.
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); validProjectID(id) {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	id, err := generateProjectID()
	if err != nil {
		return "", fmt.Errorf("generate project id: %w", err)
	}

	// Publish atomically via temp+link: build the final bytes in a
	// sibling temp file (write + fsync + close), then hard-link it onto
	// the canonical path. os.Link is no-clobber on POSIX: if the target
	// already exists the call fails with EEXIST and we read the winner.
	// Crucially, the final path is never observed in a partial state —
	// a concurrent reader either sees no file or the fully written id.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".bdtui-project-id.*.tmp")
	if err != nil {
		return "", fmt.Errorf("temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.WriteString(id); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return "", fmt.Errorf("fsync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("close temp: %w", err)
	}
	if err := os.Link(tmpName, path); err != nil {
		cleanup()
		if !os.IsExist(err) {
			return "", fmt.Errorf("link %s -> %s: %w", tmpName, path, err)
		}
		// Loser: a winner already linked the canonical path. Read it.
		winner, err := readPublishedProjectID(path)
		if err != nil {
			return "", err
		}
		return winner, nil
	}
	// We won: the canonical path now points at the same inode as our
	// temp file. Remove the temp to leave only the canonical entry.
	cleanup()
	return id, nil
}

// readPublishedProjectID polls for a non-empty file with a valid id.
// A concurrent caller may have created the file but not yet flushed the
// id bytes; a few short retries give the writer time to complete while
// keeping the per-call latency bounded.
func readPublishedProjectID(path string) (string, error) {
	const maxAttempts = 16
	delay := 200 * time.Microsecond
	for i := 0; i < maxAttempts; i++ {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("re-read %s: %w", path, err)
		}
		id := strings.TrimSpace(string(data))
		if validProjectID(id) {
			return id, nil
		}
		time.Sleep(delay)
		if delay < 2*time.Millisecond {
			delay *= 2
		}
	}
	return "", fmt.Errorf("winner %s has invalid id after %d attempts", path, maxAttempts)
}

// resolveGitDir returns the absolute path of the git directory for repoDir,
// handling normal repos, worktrees (where .git is a file pointing into
// .git/worktrees/<name>) and submodules. We use this to put the project-id
// file inside .git/ so it never appears in the tracked worktree.
func resolveGitDir(repoDir string) (string, error) {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "--absolute-git-dir")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr == "" {
			stderrStr = "exit " + err.Error()
		}
		return "", fmt.Errorf("%s", stderrStr)
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("git rev-parse returned empty path")
	}
	return out, nil
}

// generateProjectID returns a fresh 32-char hex UUIDv4 suitable for use as
// a stable opaque project id.
func generateProjectID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// RFC 4122 v4: set version + variant bits so downstream tooling can
	// recognize the id as a UUID if it ever needs to.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	out := make([]byte, 36)
	hex.Encode(out, b[:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:], b[10:])
	return strings.ReplaceAll(string(out), "-", ""), nil
}

// validProjectID returns true for non-empty ASCII hex ids of 32 chars (UUID
// without dashes) or 36 chars (with dashes). Anything else is treated as a
// corrupt file and regenerated.
func validProjectID(s string) bool {
	if len(s) != 32 && len(s) != 36 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

// snapshotWorkflow loads the named workflow bundle and compiles a canonical
// snapshot document for the daemon.
func (m model) snapshotWorkflow(ctx context.Context, name string) (workflow.Snapshot, error) {
	global, project := m.resolveWorkflowsRoots()
	loader := workflow.Loader{Global: global, Project: project}
	bundle, err := loader.Load(ctx, name)
	if err != nil {
		return workflow.Snapshot{}, fmt.Errorf("load workflow %q: %w", name, err)
	}
	return workflow.BuildSnapshot(*bundle)
}

// ensureDaemon returns a daemon client, starting one if no live socket is
// present. Returns an error if the daemon binary is unavailable.
//
// m.BeadsDir is already validated by findBeadsDir() to be the absolute
// path of the .beads directory (not its parent), so we just stat it
// directly to make sure the configured directory is still on disk at
// launch time — appending ".beads" here would look for .beads/.beads
// which never exists in a real workspace.
func (m model) ensureDaemon(ctx context.Context) (*daemon.Client, error) {
	if err := validateBeadsDir(m.BeadsDir); err != nil {
		return nil, err
	}
	opts := daemon.Options{
		SocketPath: daemonSocketPath(),
		DBPath:     daemonDBPath(m.BeadsDir),
	}
	return daemon.EnsureDaemon(ctx, opts)
}

func daemonSocketPath() string {
	if v := os.Getenv("BDTUI_DAEMON_SOCKET"); v != "" {
		return v
	}
	return daemon.DefaultSocketPath()
}

func daemonDBPath(beadsDir string) string {
	if v := os.Getenv("BDTUI_DAEMON_DB"); v != "" {
		return v
	}
	return daemon.DefaultDBPath()
}

// validateBeadsDir checks that the configured BeadsDir points at an
// existing directory. m.BeadsDir is the absolute path of the .beads
// directory itself (validated by findBeadsDir); we must NOT append
// another ".beads" here or we end up checking .beads/.beads, which
// never exists in a real workspace. Exposed for testing.
func validateBeadsDir(beadsDir string) error {
	if beadsDir == "" {
		return fmt.Errorf("no beads-dir configured")
	}
	st, err := os.Stat(beadsDir)
	if err != nil {
		return fmt.Errorf("beads-dir %q: %w", beadsDir, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("beads-dir %q is not a directory", beadsDir)
	}
	return nil
}
