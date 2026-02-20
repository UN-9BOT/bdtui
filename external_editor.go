package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	var b strings.Builder
	b.WriteString("# bdtui issue form\n\n")
	b.WriteString("## Fields\n")
	b.WriteString("- title: " + normalizeEditorScalar(payload.Title) + "\n")
	b.WriteString("- status: " + normalizeEditorScalar(payload.Status) + "\n")
	b.WriteString("- priority: " + strconv.Itoa(payload.Priority) + "\n")
	b.WriteString("- type: " + normalizeEditorScalar(payload.IssueType) + "\n")
	b.WriteString("- assignee: " + normalizeEditorScalar(payload.Assignee) + "\n")
	b.WriteString("- labels: " + normalizeEditorScalar(payload.Labels) + "\n")
	b.WriteString("- parent: " + normalizeEditorScalar(payload.Parent) + "\n\n")
	b.WriteString("## Description\n")
	if payload.Description != "" {
		b.WriteString(payload.Description)
	}
	return []byte(b.String()), nil
}

func parseEditorContent(raw []byte) (formEditorPayload, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")

	marker := "\n## Description\n"
	idx := strings.Index(text, marker)
	if idx == -1 {
		return formEditorPayload{}, fmt.Errorf("invalid editor format: expected markdown section '## Description'")
	}

	metaPart := text[:idx]
	description := text[idx+len(marker):]

	payload := formEditorPayload{Description: description}
	for _, line := range strings.Split(metaPart, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		kv := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		sep := strings.Index(kv, ":")
		if sep <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[:sep]))
		value := strings.TrimSpace(kv[sep+1:])
		switch key {
		case "title":
			payload.Title = value
		case "status":
			payload.Status = value
		case "priority":
			if value == "" {
				payload.Priority = 0
				continue
			}
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return formEditorPayload{}, fmt.Errorf("invalid priority: %q", value)
			}
			payload.Priority = parsed
		case "type":
			payload.IssueType = value
		case "assignee":
			payload.Assignee = value
		case "labels":
			payload.Labels = value
		case "parent":
			payload.Parent = value
		}
	}

	return payload, nil
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
