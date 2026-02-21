package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type TmuxTarget struct {
	SessionName string
	SessionID   string
	PaneID      string
	Command     string
	Title       string
	HasClient   bool
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
}

func newTmuxPlugin(enabled bool, runner tmuxRunner) *TmuxPlugin {
	if runner == nil {
		runner = shellTmuxRunner{}
	}
	return &TmuxPlugin{enabled: enabled, runner: runner}
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

func (p *TmuxPlugin) ListTargets() ([]TmuxTarget, error) {
	if !p.Enabled() {
		return nil, errors.New("tmux plugin disabled")
	}

	clientsRaw, err := p.runner.Run("list-clients", "-F", "#{session_id}:#{client_pid}")
	if err != nil {
		return nil, err
	}

	panesRaw, err := p.runner.Run("list-panes", "-a", "-F", "#{session_name}\t#{session_id}\t#{pane_id}\t#{pane_current_command}\t#{pane_title}")
	if err != nil {
		return nil, err
	}

	clientSessions := parseTmuxClientSessions(clientsRaw)
	targets := parseTmuxTargets(panesRaw)
	for i := range targets {
		targets[i].HasClient = clientSessions[targets[i].SessionID]
	}

	sort.SliceStable(targets, func(i, j int) bool {
		ri := rankTmuxTarget(targets[i])
		rj := rankTmuxTarget(targets[j])
		if ri != rj {
			return ri > rj
		}
		if targets[i].SessionName != targets[j].SessionName {
			return targets[i].SessionName < targets[j].SessionName
		}
		return targets[i].PaneID < targets[j].PaneID
	})

	return targets, nil
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

		parts := strings.SplitN(row, "\t", 5)
		if len(parts) < 5 {
			continue
		}

		t := TmuxTarget{
			SessionName: strings.TrimSpace(parts[0]),
			SessionID:   strings.TrimSpace(parts[1]),
			PaneID:      strings.TrimSpace(parts[2]),
			Command:     strings.TrimSpace(parts[3]),
			Title:       strings.TrimSpace(parts[4]),
		}
		if t.SessionName == "" || t.SessionID == "" || t.PaneID == "" {
			continue
		}
		out = append(out, t)
	}
	return out
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
