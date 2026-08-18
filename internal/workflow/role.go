package workflow

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkspaceMode declares whether a role may write to the worktree.
type WorkspaceMode string

const (
	WorkspaceRead  WorkspaceMode = "read"
	WorkspaceWrite WorkspaceMode = "write"
)

// Valid reports whether m is a defined workspace mode.
func (m WorkspaceMode) Valid() bool {
	return m == WorkspaceRead || m == WorkspaceWrite
}

// RoleContract is the resolved contract for a role id. It owns the prompt
// reference, allowed outcomes, declared outputs, result JSON schema, and
// workspace access mode. Role contracts resolve independently of workflows:
// a project role with a given id replaces the global role with the same id.
type RoleContract struct {
	ID          string        `yaml:"id" json:"id"`
	Description string        `yaml:"description,omitempty" json:"description,omitempty"`
	Prompt      string        `yaml:"prompt" json:"prompt"`
	Outcomes    []string      `yaml:"outcomes,omitempty" json:"outcomes,omitempty"`
	Outputs     []string      `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	ResultSchema string       `yaml:"result_schema,omitempty" json:"result_schema,omitempty"`
	Workspace   WorkspaceMode `yaml:"workspace" json:"workspace"`
}

// ParseRole decodes a role contract strictly; unknown fields are an error.
func ParseRole(data []byte) (*RoleContract, error) {
	if err := validateYAMLSubset(data); err != nil {
		return nil, fmt.Errorf("role: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var r RoleContract
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("decode role: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("decode role: multiple YAML documents")
		}
		return nil, fmt.Errorf("decode role: %w", err)
	}
	return &r, nil
}

// Validate checks the role contract fields.
func (r *RoleContract) Validate() error {
	if r == nil {
		return errors.New("role: nil contract")
	}
	if err := validateID(r.ID); err != nil {
		return fmt.Errorf("role: id: %w", err)
	}
	if err := validateRelPath(r.Prompt); err != nil {
		return fmt.Errorf("role: prompt: %w", err)
	}
	if !r.Workspace.Valid() {
		return fmt.Errorf("role: invalid workspace %q", r.Workspace)
	}
	if len(r.Outcomes) == 0 {
		return errors.New("role: at least one outcome is required")
	}
	seen := map[string]bool{}
	for _, o := range r.Outcomes {
		if strings.TrimSpace(o) == "" {
			return errors.New("role: outcome must not be empty")
		}
		if seen[o] {
			return fmt.Errorf("role: duplicate outcome %q", o)
		}
		seen[o] = true
	}
	if len(r.Outputs) == 0 {
		return errors.New("role: at least one output is required")
	}
	seenOutputs := map[string]bool{}
	for _, o := range r.Outputs {
		if strings.TrimSpace(o) == "" {
			return errors.New("role: output must not be empty")
		}
		if seenOutputs[o] {
			return fmt.Errorf("role: duplicate output %q", o)
		}
		seenOutputs[o] = true
	}
	if r.ResultSchema == "" {
		return errors.New("role: result_schema is required")
	}
	if err := validateRelPath(r.ResultSchema); err != nil {
		return fmt.Errorf("role: result_schema: %w", err)
	}
	return nil
}

// forJSON returns a copy with nil slices normalized to empty slices.
func (r RoleContract) forJSON() RoleContract {
	if r.Outcomes == nil {
		r.Outcomes = []string{}
	}
	if r.Outputs == nil {
		r.Outputs = []string{}
	}
	return r
}
