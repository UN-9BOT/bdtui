package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type TmuxTarget struct {
	SessionName string
	SessionID   string
	PaneID      string
	WindowID    string
	Command     string
	Title       string
	HasClient   bool
}

type tmuxCurrentContext struct {
	PaneID   string
	WindowID string
}

func (t TmuxTarget) Label() string {
	parts := []string{
		defaultString(strings.TrimSpace(t.SessionName), "?"),
		defaultString(strings.TrimSpace(t.PaneID), "?"),
		defaultString(strings.TrimSpace(t.Command), "-"),
	}
	if strings.TrimSpace(t.Title) != "" {
		parts = append(parts, strings.TrimSpace(t.Title))
	}
	return strings.Join(parts, " | ")
}

type tmuxRunner interface {
	Run(args ...string) (string, error)
}

type shellTmuxRunner struct{}

func (shellTmuxRunner) Run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("tmux %s failed: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("tmux %s failed: %s", strings.Join(args, " "), text)
	}
	return text, nil
}

type TmuxPlugin struct {
	enabled bool
	runner  tmuxRunner
	target  *TmuxTarget
	sleepFn func(time.Duration)
}

func newTmuxPlugin(enabled bool, runner tmuxRunner) *TmuxPlugin {
	if runner == nil {
		runner = shellTmuxRunner{}
	}
	return &TmuxPlugin{enabled: enabled, runner: runner, sleepFn: time.Sleep}
}

func (p *TmuxPlugin) SetSleepFn(fn func(time.Duration)) {
	if fn == nil {
		p.sleepFn = time.Sleep
		return
	}
	p.sleepFn = fn
}

func (p *TmuxPlugin) Enabled() bool {
	if p == nil {
		return false
	}
	return p.enabled
}

func (p *TmuxPlugin) CurrentTarget() *TmuxTarget {
	if p == nil || p.target == nil {
		return nil
	}
	clone := *p.target
	return &clone
}

func (p *TmuxPlugin) SetTarget(target TmuxTarget) {
	if p == nil {
		return
	}
	clone := target
	p.target = &clone
}

func (p *TmuxPlugin) ClearTarget() {
	if p == nil {
		return
	}
	p.target = nil
}

func (p *TmuxPlugin) ListTargets() ([]TmuxTarget, error) {
	if !p.Enabled() {
		return nil, errors.New("tmux plugin disabled")
	}

	clientsRaw, err := p.runner.Run("list-clients", "-F", "#{session_id}:#{client_pid}")
	if err != nil {
		return nil, err
	}

	panesRaw, err := p.runner.Run("list-panes", "-a", "-F", "#{session_name}\t#{session_id}\t#{pane_id}\t#{window_id}\t#{pane_current_command}\t#{pane_title}")
	if err != nil {
		return nil, err
	}

	clientSessions := parseTmuxClientSessions(clientsRaw)
	targets := parseTmuxTargets(panesRaw)
	current := p.currentContext()
	filtered := make([]TmuxTarget, 0, len(targets))
	for i := range targets {
		targets[i].HasClient = clientSessions[targets[i].SessionID]
		if current.PaneID != "" && targets[i].PaneID == current.PaneID {
			continue
		}
		filtered = append(filtered, targets[i])
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		inCurrentWindowI := current.WindowID != "" && filtered[i].WindowID == current.WindowID
		inCurrentWindowJ := current.WindowID != "" && filtered[j].WindowID == current.WindowID
		if inCurrentWindowI != inCurrentWindowJ {
			return inCurrentWindowI
		}

		ri := rankTmuxTarget(filtered[i])
		rj := rankTmuxTarget(filtered[j])
		if ri != rj {
			return ri > rj
		}
		if filtered[i].SessionName != filtered[j].SessionName {
			return filtered[i].SessionName < filtered[j].SessionName
		}
		return filtered[i].PaneID < filtered[j].PaneID
	})

	return filtered, nil
}

func (p *TmuxPlugin) SendTextToBuffer(text string) error {
	if !p.Enabled() {
		return errors.New("tmux plugin disabled")
	}
	if p.target == nil {
		return errors.New("tmux target is not selected")
	}

	payload := strings.TrimSpace(text)
	if payload == "" {
		return errors.New("empty payload")
	}

	if _, err := p.runner.Run("display-message", "-p", "-t", p.target.PaneID, "#{pane_id}"); err != nil {
		return fmt.Errorf("tmux target is unavailable: %w", err)
	}

	if _, err := p.runner.Run("set-buffer", "--", payload); err != nil {
		return err
	}
	if _, err := p.runner.Run("paste-buffer", "-t", p.target.PaneID); err != nil {
		return err
	}
	return nil
}

func (p *TmuxPlugin) SendIssueIDToBuffer(issueID string) error {
	return p.SendTextToBuffer(issueID)
}

func (p *TmuxPlugin) FocusPane(paneID string) error {
	if !p.Enabled() {
		return errors.New("tmux plugin disabled")
	}
	target := strings.TrimSpace(paneID)
	if target == "" {
		return errors.New("empty pane id")
	}
	_, err := p.runner.Run("select-pane", "-t", target)
	return err
}

func (p *TmuxPlugin) MarkPane(paneID string) error {
	if !p.Enabled() {
		return errors.New("tmux plugin disabled")
	}
	target := strings.TrimSpace(paneID)
	if target == "" {
		return errors.New("empty pane id")
	}
	_, err := p.runner.Run("select-pane", "-m", "-t", target)
	return err
}

func (p *TmuxPlugin) IsPaneMarked(paneID string) (bool, error) {
	if !p.Enabled() {
		return false, errors.New("tmux plugin disabled")
	}
	target := strings.TrimSpace(paneID)
	if target == "" {
		return false, errors.New("empty pane id")
	}
	out, err := p.runner.Run("display-message", "-p", "-t", target, "#{?pane_marked,1,0}")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "1", nil
}

func (p *TmuxPlugin) ClearMarkPane(paneID string) error {
	if !p.Enabled() {
		return errors.New("tmux plugin disabled")
	}
	target := strings.TrimSpace(paneID)
	if target == "" {
		return errors.New("empty pane id")
	}
	marked, err := p.IsPaneMarked(target)
	if err != nil {
		return err
	}
	if !marked {
		return nil
	}
	_, err = p.runner.Run("select-pane", "-M", "-t", target)
	return err
}

func (p *TmuxPlugin) BlinkPaneWindow(paneID string) error {
	if !p.Enabled() {
		return errors.New("tmux plugin disabled")
	}
	target := strings.TrimSpace(paneID)
	if target == "" {
		return errors.New("empty pane id")
	}

	windowID, err := p.runner.Run("display-message", "-p", "-t", target, "#{window_id}")
	if err != nil {
		return err
	}
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return errors.New("empty window id")
	}

	activePaneRaw, err := p.runner.Run("list-panes", "-t", windowID, "-F", "#{?pane_active,#{pane_id},}")
	if err != nil {
		return err
	}
	activePaneID := ""
	for _, line := range strings.Split(activePaneRaw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			activePaneID = line
			break
		}
	}

	oldStyle, err := p.runner.Run("show-options", "-w", "-v", "-t", windowID, "window-active-style")
	if err != nil {
		return err
	}
	oldStyle = strings.TrimSpace(oldStyle)
	if oldStyle == "" {
		oldStyle = "default"
	}

	if _, err := p.runner.Run("select-pane", "-t", target); err != nil {
		return err
	}

	steps := []string{"colour160", "default", "colour160", "default"}
	for _, bg := range steps {
		style := fmt.Sprintf("fg=default,bg=%s", bg)
		if _, err := p.runner.Run("set-option", "-w", "-t", windowID, "window-active-style", style); err != nil {
			return err
		}
		p.sleepFn(90 * time.Millisecond)
	}

	if activePaneID != "" && activePaneID != target {
		if _, err := p.runner.Run("select-pane", "-t", activePaneID); err != nil {
			return err
		}
	}

	_, err = p.runner.Run("set-option", "-w", "-t", windowID, "window-active-style", oldStyle)
	return err
}

func parseTmuxClientSessions(raw string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sessionID, _, ok := strings.Cut(line, ":")
		sessionID = strings.TrimSpace(sessionID)
		if !ok || sessionID == "" {
			continue
		}
		out[sessionID] = true
	}
	return out
}

func parseTmuxTargets(raw string) []TmuxTarget {
	rows := strings.Split(raw, "\n")
	out := make([]TmuxTarget, 0, len(rows))
	for _, row := range rows {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}

		parts := strings.SplitN(row, "\t", 6)
		if len(parts) < 5 {
			continue
		}

		t := TmuxTarget{
			SessionName: strings.TrimSpace(parts[0]),
			SessionID:   strings.TrimSpace(parts[1]),
			PaneID:      strings.TrimSpace(parts[2]),
		}
		if len(parts) >= 6 {
			t.WindowID = strings.TrimSpace(parts[3])
			t.Command = strings.TrimSpace(parts[4])
			t.Title = strings.TrimSpace(parts[5])
		} else {
			t.Command = strings.TrimSpace(parts[3])
			t.Title = strings.TrimSpace(parts[4])
		}
		if t.SessionName == "" || t.SessionID == "" || t.PaneID == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (p *TmuxPlugin) currentContext() tmuxCurrentContext {
	if p == nil {
		return tmuxCurrentContext{}
	}

	paneID := strings.TrimSpace(os.Getenv("TMUX_PANE"))
	if paneID == "" {
		return tmuxCurrentContext{}
	}

	windowID, err := p.runner.Run("display-message", "-p", "-t", paneID, "#{window_id}")
	if err != nil {
		return tmuxCurrentContext{PaneID: paneID}
	}

	return tmuxCurrentContext{
		PaneID:   paneID,
		WindowID: strings.TrimSpace(windowID),
	}
}

func rankTmuxTarget(t TmuxTarget) int {
	score := 0
	if isLikelyCodexTarget(t) {
		score += 2
	}
	if t.HasClient {
		score++
	}
	return score
}

func isLikelyCodexTarget(t TmuxTarget) bool {
	combined := strings.ToLower(strings.TrimSpace(t.Command + " " + t.Title))
	if combined == "" {
		return false
	}
	return strings.Contains(combined, "codex")
}
