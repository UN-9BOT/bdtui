package bdtui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

type formEditorPayload struct {
	Title       string
	Description string
	Status      string
	Priority    int
	IssueType   string
	Assignee    string
	Labels      string
	Parent      string
}

type formEditorFrontmatter struct {
	Title     string `yaml:"title"`
	Status    string `yaml:"status"`
	Priority  int    `yaml:"priority"`
	IssueType string `yaml:"type"`
	Assignee  string `yaml:"assignee"`
	Labels    string `yaml:"labels"`
	Parent    string `yaml:"parent"`
}

type formEditorMsg struct {
	payload formEditorPayload
	err     error
}

func (m model) openFormInEditor() (tea.Cmd, error) {
	if m.OpenFormInEditorOverride != nil {
		return m.OpenFormInEditorOverride(m)
	}
	return m.openFormInEditorCmd()
}

func (m model) openFormInEditorCmd() (tea.Cmd, error) {
	if m.Form == nil {
		return nil, fmt.Errorf("form is not active")
	}

	payload := formEditorPayload{
		Title:       m.Form.Title,
		Description: m.Form.Description,
		Status:      string(m.Form.Status),
		Priority:    m.Form.Priority,
		IssueType:   m.Form.IssueType,
		Assignee:    m.Form.Assignee,
		Labels:      m.Form.Labels,
		Parent:      m.Form.Parent,
	}

	bytes, err := marshalEditorContent(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal form for editor: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "bdtui-form-*.md")
	if err != nil {
		return nil, fmt.Errorf("create temp editor file: %w", err)
	}

	path := tmpFile.Name()
	if _, err := tmpFile.Write(bytes); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write temp editor file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close temp editor file: %w", err)
	}

	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
	}

	cmd := buildEditorCommand(editor, path)

	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		defer os.Remove(path)

		if execErr != nil {
			return formEditorMsg{err: fmt.Errorf("editor failed: %w", execErr)}
		}

		updated, err := os.ReadFile(path)
		if err != nil {
			return formEditorMsg{err: fmt.Errorf("read editor file: %w", err)}
		}

		parsed, err := parseEditorContent(updated)
		if err != nil {
			return formEditorMsg{err: err}
		}

		if parsed.Priority < 0 || parsed.Priority > 4 {
			return formEditorMsg{err: fmt.Errorf("priority must be in range 0..4")}
		}

		if parsed.Status != "" {
			if _, ok := statusFromString(parsed.Status); !ok {
				return formEditorMsg{err: fmt.Errorf("invalid status: %s", parsed.Status)}
			}
		}

		if strings.TrimSpace(parsed.IssueType) == "" {
			return formEditorMsg{err: fmt.Errorf("type must not be empty")}
		}

		return formEditorMsg{payload: parsed}
	}), nil
}

func buildEditorCommand(editor string, path string) *exec.Cmd {
	if strings.Contains(editor, " ") {
		quoted := "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
		return exec.Command("sh", "-c", editor+" "+quoted)
	}
	return exec.Command(editor, path)
}

func marshalEditorContent(payload formEditorPayload) ([]byte, error) {
	frontmatter := formEditorFrontmatter{
		Title:     normalizeEditorScalar(payload.Title),
		Status:    normalizeEditorScalar(payload.Status),
		Priority:  payload.Priority,
		IssueType: normalizeEditorScalar(payload.IssueType),
		Assignee:  normalizeEditorScalar(payload.Assignee),
		Labels:    normalizeEditorScalar(payload.Labels),
		Parent:    normalizeEditorScalar(payload.Parent),
	}

	body, err := yaml.Marshal(frontmatter)
	if err != nil {
		return nil, err
	}
	body = annotateEditorFrontmatter(body)

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(body)
	b.WriteString("---\n")
	if payload.Description != "" {
		b.WriteString(payload.Description)
	}
	return []byte(b.String()), nil
}

func annotateEditorFrontmatter(body []byte) []byte {
	annotations := map[string]string{
		"status":   strings.Join(statusValuesForEditor(), " | "),
		"priority": strings.Join(priorityValuesForEditor(), " | "),
		"type":     strings.Join(issueTypeValuesForEditor(), " | "),
	}

	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.Contains(trimmed, "#") {
			continue
		}

		for key, hint := range annotations {
			prefix := key + ":"
			if strings.HasPrefix(trimmed, prefix) {
				lines[i] = line + " # " + hint
				break
			}
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func statusValuesForEditor() []string {
	values := make([]string, 0, len(statusOrder))
	for _, status := range statusOrder {
		values = append(values, string(status))
	}
	return values
}

func priorityValuesForEditor() []string {
	values := make([]string, 0, 5)
	for priority := 0; priority <= 4; priority++ {
		values = append(values, strconv.Itoa(priority))
	}
	return values
}

func issueTypeValuesForEditor() []string {
	values := make([]string, len(issueTypes))
	copy(values, issueTypes)
	return values
}

func parseEditorContent(raw []byte) (formEditorPayload, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")

	if !strings.HasPrefix(text, "---\n") {
		return formEditorPayload{}, fmt.Errorf("invalid editor format: expected YAML frontmatter starting with ---")
	}

	rest := strings.TrimPrefix(text, "---\n")
	sep := "\n---\n"
	idx := strings.Index(rest, sep)
	if idx == -1 {
		sep = "\n---"
		idx = strings.Index(rest, sep)
		if idx == -1 {
			return formEditorPayload{}, fmt.Errorf("invalid editor format: closing frontmatter separator --- not found")
		}
	}

	yamlPart := rest[:idx]
	description := rest[idx+len(sep):]

	var frontmatter formEditorFrontmatter
	if err := yaml.Unmarshal([]byte(yamlPart), &frontmatter); err != nil {
		return formEditorPayload{}, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	return formEditorPayload{
		Title:       frontmatter.Title,
		Description: description,
		Status:      frontmatter.Status,
		Priority:    frontmatter.Priority,
		IssueType:   frontmatter.IssueType,
		Assignee:    frontmatter.Assignee,
		Labels:      frontmatter.Labels,
		Parent:      frontmatter.Parent,
	}, nil
}

func normalizeEditorScalar(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	parts := strings.Split(value, "\n")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}
