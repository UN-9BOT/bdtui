package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Bundle is the fully resolved dependency closure of a workflow at Run start:
// the workflow, its raw source YAML, the role contracts it references, and the
// raw contents of referenced prompt/schema/instruction files. Files keys are
// namespaced logical refs (roles/<id>/prompt, roles/<id>/schema, etc.).
type Bundle struct {
	Spec  WorkflowSpec
	Roles map[string]RoleContract
	Files map[string]string

	// WorkflowSource is the immutable launch-time source YAML that produced
	// Spec. It is preserved for audit/debugging, not for re-parsing.
	WorkflowSource string
}

// HumanResponseOutput is the built-in output of a human step. It makes the
// human answer an immutable output of a specific human step attempt that can
// be explicitly referenced by a later step's dataflow inputs.
const HumanResponseOutput = "response"

// Snapshot is an immutable, content-addressed workflow dependency closure.
type Snapshot struct {
	// Ref is the hex SHA-256 of JSON.
	Ref string
	// JSON is the canonical snapshot document.
	JSON string
}

// Validate performs role-aware validation on top of WorkflowSpec.Validate:
// every agent role must resolve, each step outcome must be allowed by its role
// contract, and every dataflow input must reference an output the source step
// actually declares.
func (b *Bundle) Validate() error {
	if b == nil {
		return fmt.Errorf("workflow: nil bundle")
	}
	if b.WorkflowSource == "" {
		return fmt.Errorf("workflow: workflow_source is required")
	}
	if err := b.Spec.Validate(); err != nil {
		return err
	}
	if b.Roles == nil {
		b.Roles = map[string]RoleContract{}
	}
	for id, role := range b.Roles {
		if err := role.Validate(); err != nil {
			return fmt.Errorf("workflow: role %q: %w", id, err)
		}
	}

	for i := range b.Spec.Steps {
		st := &b.Spec.Steps[i]
		if st.Type != StepAgent {
			continue
		}
		role, ok := b.Roles[st.Role]
		if !ok {
			return fmt.Errorf("workflow: step %q: role %q not found", st.ID, st.Role)
		}

		allowed := make(map[string]bool, len(role.Outcomes))
		for _, o := range role.Outcomes {
			allowed[o] = true
		}
		for _, o := range role.Outcomes {
			if _, ok := st.On[o]; !ok {
				return fmt.Errorf("workflow: step %q: outcome %q from role %q has no transition", st.ID, o, st.Role)
			}
		}
		for outcome := range st.On {
			if !allowed[outcome] {
				return fmt.Errorf("workflow: step %q: outcome %q not allowed by role %q", st.ID, outcome, st.Role)
			}
		}
	}

	for i := range b.Spec.Steps {
		st := &b.Spec.Steps[i]
		for name, ref := range st.Inputs {
			src := b.stepByID(ref.Step)
			if src == nil {
				return fmt.Errorf("workflow: step %q: input %q: step %q not found", st.ID, name, ref.Step)
			}
			if !b.declaredOutputs(src)[ref.Output] {
				return fmt.Errorf("workflow: step %q: input %q references output %q not produced by step %q", st.ID, name, ref.Output, src.ID)
			}
		}
	}

	// The snapshot must be self-sufficient: every resolved role's prompt and
	// schema content must be present in Files.
	if b.Files == nil {
		b.Files = map[string]string{}
	}
	for id, role := range b.Roles {
		if _, ok := b.Files[rolePromptKey(id)]; !ok {
			return fmt.Errorf("workflow: role %q: missing prompt dependency %q", id, rolePromptKey(id))
		}
		if role.ResultSchema != "" {
			if _, ok := b.Files[roleSchemaKey(id)]; !ok {
				return fmt.Errorf("workflow: role %q: missing schema dependency %q", id, roleSchemaKey(id))
			}
		}
	}
	return nil
}

func (b *Bundle) stepByID(id string) *StepSpec {
	for i := range b.Spec.Steps {
		if b.Spec.Steps[i].ID == id {
			return &b.Spec.Steps[i]
		}
	}
	return nil
}

func (b *Bundle) declaredOutputs(st *StepSpec) map[string]bool {
	out := map[string]bool{}
	switch st.Type {
	case StepAgent:
		if role, ok := b.Roles[st.Role]; ok {
			for _, o := range role.Outputs {
				out[o] = true
			}
		}
	case StepHuman:
		out[HumanResponseOutput] = true
	}
	return out
}

// BuildSnapshot validates the bundle and compiles a canonical, deterministic
// snapshot of the workflow, its resolved role contracts, and its dependency
// files. The ref is the hex SHA-256 of the canonical JSON.
func BuildSnapshot(b Bundle) (Snapshot, error) {
	if err := b.Validate(); err != nil {
		return Snapshot{}, err
	}
	if b.Files == nil {
		b.Files = map[string]string{}
	}
	if b.Roles == nil {
		b.Roles = map[string]RoleContract{}
	}

	roles := make(map[string]RoleContract, len(b.Roles))
	for id, r := range b.Roles {
		roles[id] = r.forJSON()
	}

	payload := struct {
		Workflow       WorkflowSpec            `json:"workflow"`
		WorkflowSource string                  `json:"workflow_source"`
		Roles          map[string]RoleContract `json:"roles"`
		Files          map[string]string       `json:"files"`
	}{
		Workflow:       b.Spec.forJSON(),
		WorkflowSource: b.WorkflowSource,
		Roles:          roles,
		Files:          b.Files,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return Snapshot{}, err
	}
	jsonStr := string(data)
	sum := sha256.Sum256([]byte(jsonStr))
	return Snapshot{Ref: hex.EncodeToString(sum[:]), JSON: jsonStr}, nil
}
