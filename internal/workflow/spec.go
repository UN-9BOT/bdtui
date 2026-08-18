// Package workflow parses, validates, and snapshots strict YAML workflow
// definitions for the orchestrator. Workflow authoring is human-driven: this
// package only reads and validates definitions; it never creates or edits
// them.
package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// StepType is the MVP step discriminator.
type StepType string

const (
	StepAgent StepType = "agent"
	StepHuman StepType = "human"
	StepEnd   StepType = "end"
)

// Valid reports whether t is a defined step type.
func (t StepType) Valid() bool {
	switch t {
	case StepAgent, StepHuman, StepEnd:
		return true
	default:
		return false
	}
}

// WorkflowSpec is the typed representation of a workflow definition.
type WorkflowSpec struct {
	Name  string     `yaml:"name" json:"name"`
	Steps []StepSpec `yaml:"steps" json:"steps"`
}

// StepSpec is a single workflow step. Fields are flat on purpose so strict
// YAML decoding can reject unknown keys; Validate enforces which fields are
// legal per step type.
type StepSpec struct {
	ID    string   `yaml:"id" json:"id"`
	Type  StepType `yaml:"type" json:"type"`
	Title string   `yaml:"title,omitempty" json:"title,omitempty"`

	// Agent step fields.
	Role         string   `yaml:"role,omitempty" json:"role,omitempty"`           // relative path to the role prompt file
	Inputs       []string `yaml:"inputs,omitempty" json:"inputs,omitempty"`       // declared input names
	Outputs      []string `yaml:"outputs,omitempty" json:"outputs,omitempty"`     // declared artifact names
	ResultSchema string   `yaml:"result_schema,omitempty" json:"result_schema,omitempty"` // relative path to a JSON Schema for result.json

	// Human step field.
	Prompt string `yaml:"prompt,omitempty" json:"prompt,omitempty"` // inline prompt text

	// Transition. Every non-end step must name its single successor.
	Next string `yaml:"next,omitempty" json:"next,omitempty"`
}

// Parse decodes a workflow definition strictly: any unknown YAML field is an
// error. Parsing does not validate the graph; call Validate separately.
func Parse(data []byte) (*WorkflowSpec, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var spec WorkflowSpec
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode workflow: %w", err)
	}
	// Reject trailing YAML documents so a file cannot smuggle extra content.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("decode workflow: multiple YAML documents")
		}
		return nil, fmt.Errorf("decode workflow: %w", err)
	}
	return &spec, nil
}

// Validate checks the workflow graph and per-step field rules. It does not
// mutate the spec.
func (s *WorkflowSpec) Validate() error {
	if s == nil {
		return errors.New("workflow: nil spec")
	}
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("workflow: name is required")
	}
	if len(s.Steps) == 0 {
		return errors.New("workflow: at least one step is required")
	}

	byID := make(map[string]int, len(s.Steps))
	for i := range s.Steps {
		st := &s.Steps[i]
		if strings.TrimSpace(st.ID) == "" {
			return fmt.Errorf("workflow: step %d: id is required", i)
		}
		if _, dup := byID[st.ID]; dup {
			return fmt.Errorf("workflow: duplicate step id %q", st.ID)
		}
		byID[st.ID] = i

		if !st.Type.Valid() {
			return fmt.Errorf("workflow: step %q: invalid type %q", st.ID, st.Type)
		}
		if err := st.validateFields(); err != nil {
			return fmt.Errorf("workflow: step %q: %w", st.ID, err)
		}
	}

	if s.Steps[0].Type == StepEnd {
		return errors.New("workflow: first step must not be end")
	}

	for i := range s.Steps {
		st := &s.Steps[i]
		switch st.Type {
		case StepEnd:
			if st.Next != "" {
				return fmt.Errorf("workflow: step %q: end step must not have next", st.ID)
			}
		default:
			if strings.TrimSpace(st.Next) == "" {
				return fmt.Errorf("workflow: step %q: missing next", st.ID)
			}
			if _, ok := byID[st.Next]; !ok {
				return fmt.Errorf("workflow: step %q: next %q not found", st.ID, st.Next)
			}
			if st.Next == st.ID {
				return fmt.Errorf("workflow: step %q: next must not be itself", st.ID)
			}
		}
	}

	return s.validateGraph()
}

func (st *StepSpec) validateFields() error {
	switch st.Type {
	case StepAgent:
		if strings.TrimSpace(st.Role) == "" {
			return errors.New("agent step requires role")
		}
		if st.Prompt != "" {
			return errors.New("agent step must not set prompt")
		}
	case StepHuman:
		if strings.TrimSpace(st.Prompt) == "" {
			return errors.New("human step requires prompt")
		}
		if st.Role != "" || st.ResultSchema != "" || len(st.Inputs) > 0 || len(st.Outputs) > 0 {
			return errors.New("human step must not set agent-only fields")
		}
	case StepEnd:
		if st.Role != "" || st.Prompt != "" || st.ResultSchema != "" || len(st.Inputs) > 0 || len(st.Outputs) > 0 {
			return errors.New("end step must not set step-specific fields")
		}
	}
	return nil
}

// validateGraph walks the single-successor graph from the entry step and
// rejects cycles and unreachable steps.
func (s *WorkflowSpec) validateGraph() error {
	byID := make(map[string]*StepSpec, len(s.Steps))
	for i := range s.Steps {
		byID[s.Steps[i].ID] = &s.Steps[i]
	}

	entry := s.Steps[0].ID
	reachable := make(map[string]bool, len(s.Steps))
	cur := entry
	for {
		if reachable[cur] {
			return fmt.Errorf("workflow: cycle detected at step %q", cur)
		}
		reachable[cur] = true
		st := byID[cur]
		if st.Type == StepEnd {
			break
		}
		cur = st.Next
	}

	for i := range s.Steps {
		if !reachable[s.Steps[i].ID] {
			return fmt.Errorf("workflow: step %q is not reachable from entry %q", s.Steps[i].ID, entry)
		}
	}
	return nil
}

// CanonicalJSON returns a deterministic, compact JSON representation of a
// validated workflow. It errors if the workflow is invalid.
func (s *WorkflowSpec) CanonicalJSON() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(s.forJSON())
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// forJSON returns a copy with nil slices normalized to empty slices so the
// canonical representation is stable regardless of how the spec was built.
func (s *WorkflowSpec) forJSON() WorkflowSpec {
	out := WorkflowSpec{Name: s.Name, Steps: make([]StepSpec, len(s.Steps))}
	for i := range s.Steps {
		st := s.Steps[i]
		if st.Inputs == nil {
			st.Inputs = []string{}
		}
		if st.Outputs == nil {
			st.Outputs = []string{}
		}
		out.Steps[i] = st
	}
	return out
}
