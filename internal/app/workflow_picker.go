package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
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

// projectIDFilename is the per-beads-dir file that stores the durable project
// identity handed to the daemon. Generated on first use and reused forever so
// the same workspace keeps the same project_id across moves, clones of other
// repos get distinct ids, and active-run uniqueness is preserved.
const projectIDFilename = ".bdtui-project-id"

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

// launchRunCmd resolves the named workflow into a snapshot, sends CreateRun
// to the daemon, then locally transitions the task to in_progress so the
// board reflects the claim without depending on bd.
//
// Run creation is the sole side-effect: we deliberately do NOT push a status
// change back to bd. The bead description states queued/running state is
// surfaced by the orchestrator, not encoded into Beads. Adding a follow-up
// bd update would split the operation into two non-atomic steps where a
// failure of the second step leaves a queued Run with no Beads claim, and a
// follow-up retry then hits ErrActiveRunExists.
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
			ProjectId:          projectID,
			TaskId:             taskID,
			WorkflowSnapshotRef: snapshot.Ref,
			WorkflowSnapshot:    snapshot.JSON,
		})
		if err != nil {
			return opMsg{err: fmt.Errorf("create run: %w", err)}
		}
		return opMsg{info: fmt.Sprintf("run %s started", run.Id)}
	}
}

// projectIDForBeadsDir returns the durable project_id stored in
// <beads-dir>/.bdtui-project-id, generating + persisting a fresh UUID hex
// (no dashes) on first use. The id is opaque, machine-independent, and
// survives moves of the workspace directory (since it lives inside it).
//
// Concurrent first calls agree on a single id: at most one caller wins the
// O_CREATE|O_EXCL create (returns its own freshly generated id); all losers
// observe IsExist and re-read the winner's id (with a tiny retry loop to
// cover the brief window between create and fsync). A corrupt persisted
// file or any other write failure surfaces as an error — we never silently
// send a daemon call with a "this-session only" id that wouldn't survive
// a restart.
//
// Returning "" with a non-nil error on a missing/unreadable beads-dir is a
// hard failure; the daemon rejects an empty project_id, so the caller must
// surface the error rather than round-trip.
func (m model) projectIDForBeadsDir() (string, error) {
	beadsDir := strings.TrimSpace(m.BeadsDir)
	if beadsDir == "" {
		return "", fmt.Errorf("no beads-dir configured")
	}
	path := filepath.Join(beadsDir, projectIDFilename)

	// Probe: read once. We never treat empty/invalid content as a hard
	// error here, because a concurrent first-time publisher may have
	// created the file but not yet flushed its bytes. Falling through to
	// the publish path keeps the file self-healing and lets concurrent
	// callers re-discover the winner via O_EXCL.
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

	// Publish atomically: O_CREATE|O_EXCL guarantees that at most one
	// concurrent caller creates the file. A loser observes IsExist, then
	// re-reads (with a tiny backoff loop so the winner has time to flush
	// its contents) and returns the winner's id. Once published, the
	// contents are stable for the lifetime of the file.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return readPublishedProjectID(path)
		}
		return "", fmt.Errorf("create %s: %w", path, err)
	}
	wrote := false
	if _, err := f.WriteString(id); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("fsync %s: %w", path, err)
	}
	if err := f.Close(); err == nil {
		wrote = true
	}
	if !wrote {
		// Close failed mid-flush; remove the partial file so a future call
		// can retry rather than seeing a corrupt id.
		_ = os.Remove(path)
		return "", fmt.Errorf("close %s: %w", path, err)
	}
	return id, nil
}

// readPublishedProjectID polls for a non-empty file with a valid id.
// A concurrent caller may have O_CREAT|O_EXCL'd the file but not yet
// flushed the id bytes; a few short retries give the writer time to
// complete while keeping the per-call latency bounded.
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
func (m model) ensureDaemon(ctx context.Context) (*daemon.Client, error) {
	if m.BeadsDir == "" {
		return nil, fmt.Errorf("no beads-dir configured")
	}
	if _, err := os.Stat(filepath.Join(m.BeadsDir, ".beads")); err != nil {
		return nil, fmt.Errorf("beads-dir %q has no .beads: %w", m.BeadsDir, err)
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