package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type MuxTarget struct {
	WorkspaceID    string
	WorkspaceLabel string
	TabID          string
	TabLabel       string
	TabNumber      int
	PaneID         string
	Agent          string
	Cwd            string
	ForegroundCwd  string
	Focused        bool
}

func (t MuxTarget) Label() string {
	scope := defaultString(strings.TrimSpace(t.WorkspaceLabel), "?")
	if tab := strings.TrimSpace(t.TabLabel); tab != "" {
		scope += "/" + tab
	}
	parts := []string{scope, defaultString(strings.TrimSpace(t.PaneID), "?")}
	if agent := strings.TrimSpace(t.Agent); agent != "" {
		parts = append(parts, agent)
	}
	cwd := strings.TrimSpace(t.ForegroundCwd)
	if cwd == "" {
		cwd = strings.TrimSpace(t.Cwd)
	}
	if cwd != "" {
		parts = append(parts, cwd)
	}
	return strings.Join(parts, " | ")
}

type herdrRunner interface {
	Run(args ...string) (string, error)
}

type shellHerdrRunner struct{}

func (shellHerdrRunner) Run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cmdArgs := append([]string{"herdr"}, args...)
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("herdr %s failed: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("herdr %s failed: %s", strings.Join(args, " "), text)
	}
	return text, nil
}

type muxBackend interface {
	Enabled() bool
	CurrentTarget() *MuxTarget
	SetTarget(MuxTarget)
	ClearTarget()
	ListTargets() ([]MuxTarget, error)
	SendTextToTarget(text string) error
	FocusTarget(target MuxTarget) error
}

type HerdrPlugin struct {
	enabled bool
	runner  herdrRunner
	target  *MuxTarget
}

func newHerdrPlugin(enabled bool, runner herdrRunner) *HerdrPlugin {
	if runner == nil {
		runner = shellHerdrRunner{}
	}
	return &HerdrPlugin{enabled: enabled, runner: runner}
}

func (p *HerdrPlugin) Enabled() bool {
	if p == nil {
		return false
	}
	return p.enabled
}

func (p *HerdrPlugin) CurrentTarget() *MuxTarget {
	if p == nil || p.target == nil {
		return nil
	}
	clone := *p.target
	return &clone
}

func (p *HerdrPlugin) SetTarget(target MuxTarget) {
	if p == nil {
		return
	}
	clone := target
	p.target = &clone
}

func (p *HerdrPlugin) ClearTarget() {
	if p == nil {
		return
	}
	p.target = nil
}

func (p *HerdrPlugin) ListTargets() ([]MuxTarget, error) {
	if !p.Enabled() {
		return nil, errors.New("herdr plugin disabled")
	}

	workspacesRaw, err := p.runner.Run("workspace", "list")
	if err != nil {
		return nil, err
	}
	tabsRaw, err := p.runner.Run("tab", "list")
	if err != nil {
		return nil, err
	}
	panesRaw, err := p.runner.Run("pane", "list")
	if err != nil {
		return nil, err
	}

	targets := parseHerdrTargets(panesRaw, tabsRaw, workspacesRaw)
	currentWorkspace := ""
	filtered := make([]MuxTarget, 0, len(targets))
	for _, target := range targets {
		if target.Focused {
			currentWorkspace = target.WorkspaceID
			continue
		}
		filtered = append(filtered, target)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		inCurrentI := currentWorkspace != "" && filtered[i].WorkspaceID == currentWorkspace
		inCurrentJ := currentWorkspace != "" && filtered[j].WorkspaceID == currentWorkspace
		if inCurrentI != inCurrentJ {
			return inCurrentI
		}
		ri := rankMuxTarget(filtered[i])
		rj := rankMuxTarget(filtered[j])
		if ri != rj {
			return ri > rj
		}
		if filtered[i].WorkspaceLabel != filtered[j].WorkspaceLabel {
			return filtered[i].WorkspaceLabel < filtered[j].WorkspaceLabel
		}
		if filtered[i].TabNumber != filtered[j].TabNumber {
			return filtered[i].TabNumber < filtered[j].TabNumber
		}
		if filtered[i].TabLabel != filtered[j].TabLabel {
			return filtered[i].TabLabel < filtered[j].TabLabel
		}
		return filtered[i].PaneID < filtered[j].PaneID
	})

	return filtered, nil
}

func (p *HerdrPlugin) SendTextToTarget(text string) error {
	if !p.Enabled() {
		return errors.New("herdr plugin disabled")
	}
	if p.target == nil {
		return errors.New("herdr target is not selected")
	}
	payload := strings.TrimSpace(text)
	if payload == "" {
		return errors.New("empty payload")
	}
	if _, err := p.runner.Run("pane", "get", p.target.PaneID); err != nil {
		return fmt.Errorf("herdr target is unavailable: %w", err)
	}
	_, err := p.runner.Run("pane", "send-text", p.target.PaneID, payload)
	return err
}

func (p *HerdrPlugin) FocusTarget(target MuxTarget) error {
	if !p.Enabled() {
		return errors.New("herdr plugin disabled")
	}
	if strings.TrimSpace(target.TabID) == "" {
		return errors.New("empty tab id")
	}
	_, err := p.runner.Run("tab", "focus", target.TabID)
	return err
}

type herdrWorkspaceList struct {
	Result struct {
		Workspaces []struct {
			WorkspaceID string `json:"workspace_id"`
			Label       string `json:"label"`
		} `json:"workspaces"`
	} `json:"result"`
}

type herdrTabList struct {
	Result struct {
		Tabs []struct {
			TabID       string `json:"tab_id"`
			WorkspaceID string `json:"workspace_id"`
			Label       string `json:"label"`
			Number      int    `json:"number"`
		} `json:"tabs"`
	} `json:"result"`
}

type herdrPaneList struct {
	Result struct {
		Panes []struct {
			PaneID        string `json:"pane_id"`
			WorkspaceID   string `json:"workspace_id"`
			TabID         string `json:"tab_id"`
			Agent         string `json:"agent"`
			Cwd           string `json:"cwd"`
			ForegroundCwd string `json:"foreground_cwd"`
			Focused       bool   `json:"focused"`
		} `json:"panes"`
	} `json:"result"`
}

func parseHerdrTargets(panesRaw, tabsRaw, workspacesRaw string) []MuxTarget {
	var workspaces herdrWorkspaceList
	if err := json.Unmarshal([]byte(workspacesRaw), &workspaces); err != nil {
		return nil
	}
	var tabs herdrTabList
	if err := json.Unmarshal([]byte(tabsRaw), &tabs); err != nil {
		return nil
	}
	var panes herdrPaneList
	if err := json.Unmarshal([]byte(panesRaw), &panes); err != nil {
		return nil
	}

	workspaceLabels := make(map[string]string, len(workspaces.Result.Workspaces))
	for _, workspace := range workspaces.Result.Workspaces {
		workspaceLabels[strings.TrimSpace(workspace.WorkspaceID)] = strings.TrimSpace(workspace.Label)
	}
	type tabMeta struct {
		label  string
		number int
	}
	tabLabels := make(map[string]tabMeta, len(tabs.Result.Tabs))
	for _, tab := range tabs.Result.Tabs {
		tabLabels[strings.TrimSpace(tab.TabID)] = tabMeta{
			label:  strings.TrimSpace(tab.Label),
			number: tab.Number,
		}
	}

	out := make([]MuxTarget, 0, len(panes.Result.Panes))
	for _, pane := range panes.Result.Panes {
		paneID := strings.TrimSpace(pane.PaneID)
		if paneID == "" {
			continue
		}
		tabID := strings.TrimSpace(pane.TabID)
		workspaceID := strings.TrimSpace(pane.WorkspaceID)
		meta := tabLabels[tabID]
		out = append(out, MuxTarget{
			WorkspaceID:    workspaceID,
			WorkspaceLabel: defaultString(workspaceLabels[workspaceID], workspaceID),
			TabID:          tabID,
			TabLabel:       meta.label,
			TabNumber:      meta.number,
			PaneID:         paneID,
			Agent:          strings.TrimSpace(pane.Agent),
			Cwd:            strings.TrimSpace(pane.Cwd),
			ForegroundCwd:  strings.TrimSpace(pane.ForegroundCwd),
			Focused:        pane.Focused,
		})
	}
	return out
}

func rankMuxTarget(target MuxTarget) int {
	score := 0
	if strings.TrimSpace(target.Agent) != "" {
		score += 2
	}
	if strings.TrimSpace(target.ForegroundCwd) != "" {
		score++
	}
	return score
}

func parseHerdrTargetsFromLines(lines []string) []MuxTarget {
	if len(lines) < 3 {
		return nil
	}
	return parseHerdrTargets(lines[0], lines[1], lines[2])
}

func parseTabNumber(raw string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	return n
}
