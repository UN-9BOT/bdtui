package main

import (
	"fmt"
	"os"
	"os/exec"
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

func (m model) openFormInEditorCmd() (tea.Cmd, error) {
	if m.form == nil {
		return nil, fmt.Errorf("form is not active")
	}

	payload := formEditorPayload{
		Title:       m.form.Title,
		Description: m.form.Description,
		Status:      string(m.form.Status),
		Priority:    m.form.Priority,
		IssueType:   m.form.IssueType,
		Assignee:    m.form.Assignee,
		Labels:      m.form.Labels,
		Parent:      m.form.Parent,
	}

	frontmatter := formEditorFrontmatter{
		Title:     payload.Title,
		Status:    payload.Status,
		Priority:  payload.Priority,
		IssueType: payload.IssueType,
		Assignee:  payload.Assignee,
		Labels:    payload.Labels,
		Parent:    payload.Parent,
	}

	bytes, err := marshalEditorContent(frontmatter, payload.Description)
	if err != nil {
		return nil, fmt.Errorf("marshal form for editor: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "bdtui-form-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create temp editor file: %w", err)
	}

	path := tmpFile.Name()
	if _, err := tmpFile.Write(append(bytes, '\n')); err != nil {
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

func marshalEditorContent(frontmatter formEditorFrontmatter, description string) ([]byte, error) {
	body, err := yaml.Marshal(frontmatter)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(body)
	b.WriteString("---\n")
	if description != "" {
		b.WriteString(description)
		if !strings.HasSuffix(description, "\n") {
			b.WriteString("\n")
		}
	}
	return []byte(b.String()), nil
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
	description = strings.TrimPrefix(description, "\n")
	description = strings.TrimRight(description, "\n")

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
