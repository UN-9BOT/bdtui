package bdtui

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type BdClient struct {
	RepoDir string
}

const sortModeKVKey = "bdtui.sort_mode"

func NewBdClient(repoDir string) *BdClient {
	return &BdClient{RepoDir: repoDir}
}

type rawDependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
}

type rawIssue struct {
	ID           any             `json:"id"`
	Title        any             `json:"title"`
	Description  any             `json:"description"`
	Status       any             `json:"status"`
	Priority     any             `json:"priority"`
	IssueType    any             `json:"issue_type"`
	Assignee     any             `json:"assignee"`
	Labels       any             `json:"labels"`
	CreatedAt    any             `json:"created_at"`
	UpdatedAt    any             `json:"updated_at"`
	ClosedAt     any             `json:"closed_at"`
	Parent       any             `json:"parent"`
	Dependencies []rawDependency `json:"dependencies"`
}

type CreateParams struct {
	Title       string
	Description string
	Priority    int
	IssueType   string
	Assignee    string
	Labels      []string
	Parent      string
}

type UpdateParams struct {
	ID          string
	Title       *string
	Description *string
	Status      *Status
	Priority    *int
	IssueType   *string
	Assignee    *string
	Labels      *[]string
	Parent      *string
}

func (c *BdClient) run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", args...)
	cmd.Dir = c.RepoDir

	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("bd %s failed: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("bd %s failed: %s", strings.Join(args, " "), text)
	}
	return text, nil
}

func (c *BdClient) ListIssues() ([]Issue, string, error) {
	out, err := c.run("list", "--json", "--all", "-n", "0")
	if err != nil {
		return nil, "", err
	}

	hashSum := sha1.Sum([]byte(out))
	hash := hex.EncodeToString(hashSum[:])

	var raw []rawIssue
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, "", fmt.Errorf("parse bd list json: %w", err)
	}

	issues := normalizeIssues(raw)
	return issues, hash, nil
}

func normalizeIssues(raw []rawIssue) []Issue {
	issues := make([]Issue, 0, len(raw))
	byID := make(map[string]*Issue, len(raw))

	for _, r := range raw {
		id := asString(r.ID)
		title := asString(r.Title)
		if id == "" || title == "" {
			continue
		}

		st, ok := statusFromString(asString(r.Status))
		if !ok {
			st = StatusOpen
		}

		issue := Issue{
			ID:          id,
			Title:       title,
			Description: asString(r.Description),
			Status:      st,
			Display:     st,
			Priority:    clampPriority(asInt(r.Priority, 2)),
			IssueType:   defaultString(asString(r.IssueType), "task"),
			Assignee:    asString(r.Assignee),
			Labels:      asStringSlice(r.Labels),
			CreatedAt:   asString(r.CreatedAt),
			UpdatedAt:   asString(r.UpdatedAt),
			ClosedAt:    asString(r.ClosedAt),
			Parent:      asString(r.Parent),
		}

		issues = append(issues, issue)
	}

	for i := range issues {
		byID[issues[i].ID] = &issues[i]
	}

	for _, r := range raw {
		issueID := asString(r.ID)
		issue := byID[issueID]
		if issue == nil {
			continue
		}

		for _, dep := range r.Dependencies {
			switch dep.Type {
			case "parent-child":
				if dep.DependsOnID != "" {
					issue.Parent = dep.DependsOnID
					if parent := byID[dep.DependsOnID]; parent != nil {
						parent.Children = appendUnique(parent.Children, issue.ID)
					}
				}
			case "blocks":
				if dep.DependsOnID != "" {
					issue.BlockedBy = appendUnique(issue.BlockedBy, dep.DependsOnID)
					if blocker := byID[dep.DependsOnID]; blocker != nil {
						blocker.Blocks = appendUnique(blocker.Blocks, issue.ID)
					}
				}
			}
		}
	}

	for i := range issues {
		issue := &issues[i]
		active := issue.BlockedBy[:0]
		for _, blockerID := range issue.BlockedBy {
			blocker := byID[blockerID]
			if blocker == nil || blocker.Status == StatusClosed {
				continue
			}
			active = append(active, blockerID)
		}
		issue.BlockedBy = active
		if issue.Status == StatusOpen && len(issue.BlockedBy) > 0 {
			issue.Display = StatusBlocked
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Priority != issues[j].Priority {
			return issues[i].Priority < issues[j].Priority
		}
		if issues[i].UpdatedAt != issues[j].UpdatedAt {
			return issues[i].UpdatedAt > issues[j].UpdatedAt
		}
		return issues[i].ID < issues[j].ID
	})

	return issues
}

func (c *BdClient) CreateIssue(p CreateParams) (string, error) {
	args := []string{"create", p.Title, "--silent"}
	if p.Description != "" {
		args = append(args, "-d", p.Description)
	}
	args = append(args, "-p", strconv.Itoa(clampPriority(p.Priority)))
	if p.IssueType != "" {
		args = append(args, "-t", p.IssueType)
	}
	if p.Assignee != "" {
		args = append(args, "-a", p.Assignee)
	}
	if len(p.Labels) > 0 {
		args = append(args, "-l", strings.Join(p.Labels, ","))
	}
	if p.Parent != "" {
		args = append(args, "--parent", p.Parent)
	}
	out, err := c.run(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *BdClient) UpdateIssue(p UpdateParams) error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("update requires issue id")
	}

	args := []string{"update", p.ID}
	if p.Title != nil {
		args = append(args, "--title", *p.Title)
	}
	if p.Description != nil {
		args = append(args, "-d", *p.Description)
	}
	if p.Status != nil {
		args = append(args, "-s", string(*p.Status))
	}
	if p.Priority != nil {
		args = append(args, "-p", strconv.Itoa(clampPriority(*p.Priority)))
	}
	if p.IssueType != nil {
		args = append(args, "-t", *p.IssueType)
	}
	if p.Assignee != nil {
		args = append(args, "-a", *p.Assignee)
	}
	if p.Parent != nil {
		args = append(args, "--parent", *p.Parent)
	}
	if p.Labels != nil {
		labelsArg := strings.Join(*p.Labels, ",")
		args = append(args, "--set-labels", labelsArg)
	}

	_, err := c.run(args...)
	return err
}

func (c *BdClient) CloseIssue(id string) error {
	_, err := c.run("close", id)
	return err
}

func (c *BdClient) ReopenIssue(id string) error {
	_, err := c.run("reopen", id)
	return err
}

func (c *BdClient) DeletePreview(id string) (string, error) {
	return c.run("delete", id, "--dry-run")
}

func (c *BdClient) DeleteIssue(id string, mode DeleteMode) error {
	args := []string{"delete", id, "--force"}
	if mode == DeleteModeCascade {
		args = append(args, "--cascade")
	}
	_, err := c.run(args...)
	return err
}

func (c *BdClient) DepAdd(blockedID, blockerID string) error {
	_, err := c.run("dep", "add", blockedID, blockerID)
	return err
}

func (c *BdClient) DepRemove(blockedID, blockerID string) error {
	_, err := c.run("dep", "remove", blockedID, blockerID)
	return err
}

func (c *BdClient) DepList(issueID string) (string, error) {
	return c.run("dep", "list", issueID)
}

func (c *BdClient) SetParent(id, parent string) error {
	_, err := c.run("update", id, "--parent", parent)
	return err
}

func (c *BdClient) GetSortMode() (SortMode, error) {
	out, err := c.run("kv", "get", sortModeKVKey)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "(not set)") {
			return SortModeStatusDateOnly, nil
		}
		return SortModeStatusDateOnly, err
	}

	mode, ok := parseSortMode(out)
	if !ok {
		return SortModeStatusDateOnly, nil
	}
	return mode, nil
}

func (c *BdClient) SetSortMode(mode SortMode) error {
	if _, ok := parseSortMode(string(mode)); !ok {
		mode = SortModeStatusDateOnly
	}
	_, err := c.run("kv", "set", sortModeKVKey, string(mode))
	return err
}

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		return strconv.Itoa(int(t))
	case int:
		return strconv.Itoa(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func asInt(v any, fallback int) int {
	switch t := v.(type) {
	case nil:
		return fallback
	case int:
		return t
	case float64:
		return int(t)
	case json.Number:
		n, err := t.Int64()
		if err == nil {
			return int(n)
		}
	case string:
		if t == "" {
			return fallback
		}
		if strings.HasPrefix(strings.ToLower(t), "p") {
			t = t[1:]
		}
		n, err := strconv.Atoi(t)
		if err == nil {
			return n
		}
	}
	return fallback
}

func asStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s := asString(item)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return parseLabels(t)
	default:
		return nil
	}
}

func defaultString(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func clampPriority(v int) int {
	if v < 0 {
		return 0
	}
	if v > 4 {
		return 4
	}
	return v
}

func appendUnique(xs []string, val string) []string {
	if val == "" {
		return xs
	}
	for _, x := range xs {
		if x == val {
			return xs
		}
	}
	return append(xs, val)
}

func projectRootFromBeads(beadsDir string) string {
	return filepath.Dir(beadsDir)
}
