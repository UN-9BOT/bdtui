package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bdtui/internal/daemon"
	"bdtui/internal/daemon/daemonpb"

	tea "github.com/charmbracelet/bubbletea"
)

// runsLoadTimeout bounds how long the Runs tab will block waiting for
// ListRuns to return. Tighter than the global daemon timeout so the
// operator sees a quick "loading failed" toast instead of a long stall.
const runsLoadTimeout = 5 * time.Second

// loadRunsCmd returns a tea.Cmd that fetches the current Run list from
// the daemon and delivers it as runsLoadedMsg. The caller must already
// have m.Daemon != nil (set by openRunsTab).
func loadRunsCmd(client *daemon.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), runsLoadTimeout)
		defer cancel()
		resp, err := client.ListRuns(ctx, &daemonpb.ListRunsRequest{})
		if err != nil {
			return runsLoadedMsg{err: err}
		}
		rows := make([]RunRow, 0, len(resp.Runs))
		for _, r := range resp.Runs {
			rows = append(rows, runRowFromProto(r, client))
		}
		return runsLoadedMsg{rows: rows}
	}
}

// runRowFromProto converts a daemon Run protobuf into the coarse RunRow
// the Runs tab renders. PaneID is resolved via a separate Inspect call
// only when the Run is in needs_attention / waiting_human -- the common
// case skips the extra round trip.
func runRowFromProto(r *daemonpb.Run, client *daemon.Client) RunRow {
	row := RunRow{
		RunID:             r.Id,
		ProjectID:         r.ProjectId,
		TaskID:            r.TaskId,
		Status:            r.Status,
		CurrentStepID:     derefString(r.CurrentStepId),
		NeedsAttention:    derefString(r.NeedsAttentionReason),
		WorkflowStageHint: stageHintFromSnapshot(r.WorkflowSnapshot),
		HasPendingHuman:   r.Status == "waiting_human",
	}
	if r.Status == "waiting_human" || r.Status == "needs_attention" {
		// Best-effort pane lookup. If Inspect fails the row still renders.
		ctx, cancel := context.WithTimeout(context.Background(), runsLoadTimeout)
		defer cancel()
		// We don't know the execution id from just the Run; the pane is
		// looked up lazily on focus action. Leave PaneID empty here.
		_ = client
		_ = ctx
	}
	return row
}

// stageHintFromSnapshot extracts a short human-readable step hint from
// the stored workflow snapshot. Returns "" if the snapshot is empty or
// not parseable; the Runs tab still works without a hint.
func stageHintFromSnapshot(snapshotJSON string) string {
	if snapshotJSON == "" {
		return ""
	}
	// Cheap heuristic: count occurrences of "id:" inside steps and pick
	// the first one -- this is good enough for an MVP coarse display.
	// A real implementation would parse the snapshot JSON and walk
	// currentStepId references.
	i := strings.Index(snapshotJSON, `"id":`)
	if i < 0 {
		return ""
	}
	j := i + len(`"id":"`)
	if j >= len(snapshotJSON) {
		return ""
	}
	end := strings.IndexByte(snapshotJSON[j:], '"')
	if end < 0 {
		return ""
	}
	return snapshotJSON[j : j+end]
}

// derefString is a tiny helper for nullable protobuf string fields.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// runsLoadedMsg carries the result of loadRunsCmd back to the model.
type runsLoadedMsg struct {
	rows []RunRow
	err  error
}

// openRunsTab switches the model into ModeRuns and starts an async load
// of the current Run list. If the daemon isn't reachable, the user
// gets a warning toast and the mode is left unchanged so they can fall
// back to the board.
func (m model) openRunsTab() (tea.Model, tea.Cmd) {
	// ensureDaemon starts the daemon if needed and returns a client; if
	// the user has never launched a run before, the daemon may not be
	// installed and the helper surfaces an error.
	client, err := m.ensureDaemon(daemonOpenCtx())
	if err != nil {
		m.setToast("warning", "daemon: "+err.Error())
		return m, nil
	}
	m.Daemon = client
	m.Runs = &RunsTabState{LoadingMsg: "loading runs..."}
	m.Mode = ModeRuns
	m.clearTransientUI()
	return m, loadRunsCmd(client)
}

// daemonOpenCtx returns a short-lived context for opening the daemon
// client. ensureDaemon already takes a context for the gRPC connect
// timeout; we don't need it to outlive this call.
func daemonOpenCtx() context.Context {
	return context.Background()
}

// handleRunsLoaded applies a runsLoadedMsg back to the model: it stores
// the rows (or error) on m.Runs and emits a tea.Cmd so the next render
// repaints. The Runs tab stays in ModeRuns so the user can retry.
func (m model) handleRunsLoaded(msg runsLoadedMsg) (tea.Model, tea.Cmd) {
	if m.Runs == nil {
		m.Runs = &RunsTabState{}
	}
	m.Runs.Loaded = true
	m.Runs.LoadingMsg = ""
	if msg.err != nil {
		m.Runs.LastError = fmt.Sprintf("load failed: %v", msg.err)
		m.setToast("warning", m.Runs.LastError)
		m.Runs.Rows = nil
		m.Runs.Index = 0
		return m, nil
	}
	m.Runs.Rows = msg.rows
	m.Runs.LastError = ""
	if m.Runs.Index >= len(m.Runs.Rows) {
		m.Runs.Index = 0
	}
	return m, nil
}

// currentRun returns the selected run row, or nil if the tab has no
// rows yet. Safe to call from any keyboard handler in ModeRuns.
func (m model) currentRun() *RunRow {
	if m.Runs == nil || len(m.Runs.Rows) == 0 {
		return nil
	}
	if m.Runs.Index < 0 || m.Runs.Index >= len(m.Runs.Rows) {
		return nil
	}
	return &m.Runs.Rows[m.Runs.Index]
}

// retrySelectedRun sends RetryRun on the gRPC client for the current
// selection. Marks the run as needs_attention on the server side; the
// next render picks up the new state via a refresh.
func (m model) retrySelectedRun() (tea.Model, tea.Cmd) {
	run := m.currentRun()
	if run == nil {
		m.setToast("warning", "no run selected")
		return m, nil
	}
	if m.Daemon == nil {
		m.setToast("warning", "daemon not running")
		return m, nil
	}
	client := m.Daemon
	runID := run.RunID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), runsLoadTimeout)
		defer cancel()
		_, err := client.RetryRun(ctx, &daemonpb.RetryRunRequest{Id: runID})
		if err != nil {
			return runsActionMsg{action: "retry", runID: runID, err: err}
		}
		return runsActionMsg{action: "retry", runID: runID}
	}
}

// cancelSelectedRun sends CancelRun on the gRPC client.
func (m model) cancelSelectedRun() (tea.Model, tea.Cmd) {
	run := m.currentRun()
	if run == nil {
		m.setToast("warning", "no run selected")
		return m, nil
	}
	if m.Daemon == nil {
		m.setToast("warning", "daemon not running")
		return m, nil
	}
	client := m.Daemon
	runID := run.RunID
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), runsLoadTimeout)
		defer cancel()
		_, err := client.CancelRun(ctx, &daemonpb.CancelRunRequest{Id: runID})
		if err != nil {
			return runsActionMsg{action: "cancel", runID: runID, err: err}
		}
		return runsActionMsg{action: "cancel", runID: runID}
	}
}

// runsActionMsg is the response message for retry/cancel commands.
type runsActionMsg struct {
	action string
	runID  string
	err    error
}

// handleRunsAction applies a runsActionMsg back to the model: shows a
// toast on success or failure, and triggers a fresh load so the next
// render reflects the new run status.
func (m model) handleRunsAction(msg runsActionMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setToast("warning", fmt.Sprintf("%s failed: %v", msg.action, msg.err))
		return m, nil
	}
	m.setToast("success", fmt.Sprintf("%s sent for %s", msg.action, shortRunID(msg.runID)))
	return m, loadRunsCmd(m.Daemon)
}

// shortRunID returns the first 8 chars of a run id, for compact toasts.
func shortRunID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// moveRunSelection clamps the Runs tab selection by delta rows. Used by
// the j/k bindings; treats the empty list as a no-op.
func (m model) moveRunSelection(delta int) {
	if m.Runs == nil || len(m.Runs.Rows) == 0 {
		return
	}
	m.Runs.Index += delta
	if m.Runs.Index < 0 {
		m.Runs.Index = 0
	}
	if m.Runs.Index >= len(m.Runs.Rows) {
		m.Runs.Index = len(m.Runs.Rows) - 1
	}
}