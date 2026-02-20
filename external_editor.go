package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type formEditorPayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"type"`
	Assignee    string `json:"assignee"`
	Labels      string `json:"labels"`
	Parent      string `json:"parent"`
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

	bytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal form for editor: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "bdtui-form-*.json")
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

		var parsed formEditorPayload
		if err := json.Unmarshal(updated, &parsed); err != nil {
			return formEditorMsg{err: fmt.Errorf("invalid JSON in editor file: %w", err)}
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
