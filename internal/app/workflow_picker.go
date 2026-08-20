package app

import (
	"context"
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

		run, err := client.CreateRun(ctx, &daemonpb.CreateRunRequest{
			TaskId:             taskID,
			WorkflowSnapshotRef: snapshot.Ref,
			WorkflowSnapshot:    snapshot.JSON,
		})
		if err != nil {
			return opMsg{err: fmt.Errorf("create run: %w", err)}
		}

		// Mark the task as locally claimed. The board reads Display +
		// Status so this surfaces the active run without writing back to bd.
		if err := m.Client.UpdateIssue(UpdateParams{
			ID:     taskID,
			Status: statusPtr(StatusInProgress),
		}); err != nil {
			return opMsg{err: fmt.Errorf("claim: %w", err), info: fmt.Sprintf("run %s created", run.Id)}
		}
		return opMsg{info: fmt.Sprintf("run %s started", run.Id)}
	}
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

func statusPtr(s Status) *Status { return &s }