// Package workflow parses, validates, resolves, and snapshots strict YAML
// workflow definitions and their role contracts for the orchestrator.
// Workflow authoring is human-driven: this package only reads and validates
// definitions; it never creates or edits them.
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

// CurrentVersion is the workflow format version this package understands.
const CurrentVersion = 1

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

// InputRef is an explicit dataflow reference: it names a step and one of that
// step's declared outputs. It is not a bare global name and not a filesystem
// convention.
type InputRef struct {
	Step   string `yaml:"step" json:"step"`
	Output string `yaml:"output" json:"output"`
}

// WorkflowSpec is the typed representation of a workflow definition.
type WorkflowSpec struct {
	Version int        `yaml:"version" json:"version"`
	Name    string     `yaml:"name" json:"name"`
	Steps   []StepSpec `yaml:"steps" json:"steps"`
}

// StepSpec is a single workflow step.
//
// Transitions are semantic: `on` maps a role/human outcome to the next step
// id. Technical failures are handled by the engine, not encoded here. Cycles
// are legal (e.g. plan -> review -> revise -> plan).
type StepSpec struct {
	ID    string   `yaml:"id" json:"id"`
	Type  StepType `yaml:"type" json:"type"`
	Title string   `yaml:"title,omitempty" json:"title,omitempty"`

	// Role is a role id (not a path). The role contract owns the prompt,
	// allowed outcomes, result schema, declared outputs, and workspace mode.
	Role string `yaml:"role,omitempty" json:"role,omitempty"`

	// Inputs is a dataflow map from a local input name to its source
	// (step, output).
	Inputs map[string]InputRef `yaml:"inputs,omitempty" json:"inputs,omitempty"`

	// On maps a semantic outcome to the next step id.
	On map[string]string `yaml:"on,omitempty" json:"on,omitempty"`

	// Prompt is an optional static prompt for human steps. It supplements
	// inputs; it never replaces explicit dataflow.
	Prompt string `yaml:"prompt,omitempty" json:"prompt,omitempty"`
}

// Parse decodes a workflow definition strictly: any unknown YAML field is an
// error. It does not validate the graph; call Validate separately.
func Parse(data []byte) (*WorkflowSpec, error) {
	if err := validateYAMLSubset(data); err != nil {
		return nil, fmt.Errorf("workflow: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var spec WorkflowSpec
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode workflow: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("decode workflow: multiple YAML documents")
		}
		return nil, fmt.Errorf("decode workflow: %w", err)
	}
	return &spec, nil
}

// Validate checks the workflow graph and per-step field rules. It allows
// cycles but rejects unreachable steps, missing transition targets, and
// malformed dataflow references. Role-contract-level checks happen after role
// resolution (see Bundle.Validate).
func (s *WorkflowSpec) Validate() error {
	if s == nil {
		return errors.New("workflow: nil spec")
	}
	if s.Version != CurrentVersion {
		return fmt.Errorf("workflow: unsupported version %d (want %d)", s.Version, CurrentVersion)
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
		for outcome, target := range st.On {
			if strings.TrimSpace(outcome) == "" {
				return fmt.Errorf("workflow: step %q: empty outcome", st.ID)
			}
			if _, ok := byID[target]; !ok {
				return fmt.Errorf("workflow: step %q: outcome %q target %q not found", st.ID, outcome, target)
			}
		}
		for name, ref := range st.Inputs {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("workflow: step %q: empty input name", st.ID)
			}
			if strings.TrimSpace(ref.Step) == "" {
				return fmt.Errorf("workflow: step %q: input %q: step is required", st.ID, name)
			}
			if strings.TrimSpace(ref.Output) == "" {
				return fmt.Errorf("workflow: step %q: input %q: output is required", st.ID, name)
			}
			idx, ok := byID[ref.Step]
			if !ok {
				return fmt.Errorf("workflow: step %q: input %q: step %q not found", st.ID, name, ref.Step)
			}
			if s.Steps[idx].Type == StepEnd {
				return fmt.Errorf("workflow: step %q: input %q: step %q is an end step and produces no output", st.ID, name, ref.Step)
			}
		}
	}

	if err := s.validateGraph(); err != nil {
		return err
	}
	return s.validateDataflowDominance()
}

func (st *StepSpec) validateFields() error {
	switch st.Type {
	case StepAgent:
		if err := validateID(st.Role); err != nil {
			return fmt.Errorf("agent step role: %w", err)
		}
		if st.Prompt != "" {
			return errors.New("agent step must not set prompt")
		}
		if len(st.On) == 0 {
			return errors.New("agent step requires at least one outcome in on")
		}
	case StepHuman:
		if st.Role != "" {
			return errors.New("human step must not set role")
		}
		if len(st.On) == 0 {
			return errors.New("human step requires at least one outcome in on")
		}
	case StepEnd:
		if st.Role != "" || st.Prompt != "" || len(st.Inputs) > 0 || len(st.On) > 0 {
			return errors.New("end step must not set role, prompt, inputs, or on")
		}
	}
	return nil
}

// validateGraph walks the outcome graph from the entry step and rejects
// unreachable steps. Cycles are legal and are not an error.
func (s *WorkflowSpec) validateGraph() error {
	byID := make(map[string]*StepSpec, len(s.Steps))
	for i := range s.Steps {
		byID[s.Steps[i].ID] = &s.Steps[i]
	}

	reachable := make(map[string]bool, len(s.Steps))
	var visit func(id string)
	visit = func(id string) {
		if reachable[id] {
			return
		}
		reachable[id] = true
		for _, target := range byID[id].On {
			visit(target)
		}
	}
	visit(s.Steps[0].ID)

	for i := range s.Steps {
		if !reachable[s.Steps[i].ID] {
			return fmt.Errorf("workflow: step %q is not reachable from entry %q", s.Steps[i].ID, s.Steps[0].ID)
		}
	}
	return nil
}

// validateDataflowDominance enforces that a required input's source step must
// be guaranteed to have executed before its consumer: the source must dominate
// the consumer in the control-flow graph (every path from the entry to the
// consumer passes through the source).
func (s *WorkflowSpec) validateDataflowDominance() error {
	adj := make(map[string][]string, len(s.Steps))
	for i := range s.Steps {
		for _, target := range s.Steps[i].On {
			adj[s.Steps[i].ID] = append(adj[s.Steps[i].ID], target)
		}
	}

	entry := s.Steps[0].ID
	for i := range s.Steps {
		consumer := &s.Steps[i]
		for name, ref := range consumer.Inputs {
			if ref.Step == consumer.ID {
				return fmt.Errorf("workflow: step %q: input %q: source must not be itself", consumer.ID, name)
			}
			if !dominates(entry, ref.Step, consumer.ID, adj) {
				return fmt.Errorf("workflow: step %q: input %q: source step %q does not dominate consumer", consumer.ID, name, ref.Step)
			}
		}
	}
	return nil
}

// dominates reports whether every path from entry to c passes through s.
func dominates(entry, s, c string, adj map[string][]string) bool {
	// s dominates c iff c is NOT reachable from entry while avoiding s.
	visited := map[string]bool{}
	var dfs func(id string) bool
	dfs = func(id string) bool {
		if id == c {
			return true
		}
		if id == s {
			return false
		}
		if visited[id] {
			return false
		}
		visited[id] = true
		for _, nxt := range adj[id] {
			if dfs(nxt) {
				return true
			}
		}
		return false
	}
	return !dfs(entry)
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

// forJSON returns a copy with nil maps normalized to empty maps so the
// canonical representation is stable regardless of how the spec was built.
func (s *WorkflowSpec) forJSON() WorkflowSpec {
	out := WorkflowSpec{Version: s.Version, Name: s.Name, Steps: make([]StepSpec, len(s.Steps))}
	for i := range s.Steps {
		st := s.Steps[i]
		if st.Inputs == nil {
			st.Inputs = map[string]InputRef{}
		}
		if st.On == nil {
			st.On = map[string]string{}
		}
		out.Steps[i] = st
	}
	return out
}
