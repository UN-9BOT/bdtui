package agent

import (
	"strings"
	"testing"

	"bdtui/internal/workflow"
)

// validEnvelopeInput returns a fully-populated EnvelopeInput used as the base
// for envelope tests. Tests mutate fields off this base.
func validEnvelopeInput() EnvelopeInput {
	return EnvelopeInput{
		Role: workflow.RoleContract{
			ID:           "planner",
			Description:  "Plans the work",
			Workspace:    workflow.WorkspaceWrite,
			Outcomes:     []string{"planned", "needs_clarification"},
			Outputs:      []string{"plan", "alternatives"},
			ResultSchema: `{"type":"object","required":["outcome","plan"],"properties":{"outcome":{"type":"string","enum":["planned","needs_clarification"]},"plan":{"type":"string"}}}`,
		},
		RolePrompt: "Produce a plan for the task.",
		Task: TaskSnapshot{
			ID:          "bdtui-1",
			Title:       "Implement feature X",
			Description: "Ship feature X behind a flag.",
		},
		Instructions: []ProjectInstruction{
			{Name: "AGENTS.md", Content: "Use bd for tracking."},
			{Name: ".claude/CLAUDE.md", Content: "Prefer small PRs."},
		},
		Inputs: map[string]any{
			"clarification": "from human step",
			"prior":         map[string]any{"v": 1, "ok": true},
		},
		OutputPaths: OutputPaths{
			Result:    "/run/storage/result.json",
			Artifacts: map[string]string{"plan": "/run/storage/plan.md"},
		},
		Contract: ResultContract{
			Schema:          `{"type":"object","required":["outcome","plan"],"properties":{"outcome":{"type":"string","enum":["planned","needs_clarification"]},"plan":{"type":"string"}}}`,
			AllowedOutcomes: []string{"planned", "needs_clarification"},
		},
	}
}

func TestBuildEnvelopeDeterministic(t *testing.T) {
	in := validEnvelopeInput()
	a, err := BuildEnvelope(in)
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	b, err := BuildEnvelope(in)
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	if a != b {
		t.Fatalf("BuildEnvelope is not deterministic")
	}
}

func TestBuildEnvelopeSortsInputs(t *testing.T) {
	in := validEnvelopeInput()
	a, _ := BuildEnvelope(in)

	in.Inputs = map[string]any{"zzz": 1, "aaa": 2, "mmm": 3}
	b, _ := BuildEnvelope(in)

	if a == b {
		t.Fatalf("expected sorted-key outputs to differ from unsorted-input renders only if the order changed; check sort")
	}

	// Build two envelopes with same set, different insertion order; both must
	// be equal.
	in1 := validEnvelopeInput()
	in1.Inputs = map[string]any{"a": 1, "b": 2}
	in2 := validEnvelopeInput()
	in2.Inputs = map[string]any{"b": 2, "a": 1}
	x, _ := BuildEnvelope(in1)
	y, _ := BuildEnvelope(in2)
	if x != y {
		t.Fatalf("BuildEnvelope must render inputs in sorted-key order regardless of Go map iteration")
	}
}

func TestBuildEnvelopePreservesInstructionOrder(t *testing.T) {
	in := validEnvelopeInput()
	got, err := BuildEnvelope(in)
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	// First instruction must precede the second in the rendered output.
	i1 := strings.Index(got, "## AGENTS.md")
	i2 := strings.Index(got, "## .claude/CLAUDE.md")
	if i1 < 0 || i2 < 0 || i1 > i2 {
		t.Fatalf("instructions not in declared order: AGENTS=%d claude=%d", i1, i2)
	}
}

func TestBuildEnvelopeRequiresFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*EnvelopeInput)
	}{
		{"no role id", func(in *EnvelopeInput) { in.Role.ID = "" }},
		{"no role prompt", func(in *EnvelopeInput) { in.RolePrompt = "" }},
		{"no result path", func(in *EnvelopeInput) { in.OutputPaths.Result = "" }},
		{"no outcomes", func(in *EnvelopeInput) { in.Contract.AllowedOutcomes = nil }},
		{"no schema", func(in *EnvelopeInput) { in.Contract.Schema = "" }},
		{"blank instruction name", func(in *EnvelopeInput) {
			in.Instructions = []ProjectInstruction{{Name: "", Content: "x"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validEnvelopeInput()
			tc.mutate(&in)
			if _, err := BuildEnvelope(in); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestBuildEnvelopeContainsOutputPaths(t *testing.T) {
	got, err := BuildEnvelope(validEnvelopeInput())
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	for _, needle := range []string{
		"/run/storage/result.json",
		"/run/storage/plan.md",
		"declared_outputs:",
		"allowed_outcomes:",
		"# Output Contract",
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("envelope missing %q\n---\n%s", needle, got)
		}
	}
}

func TestBuildEnvelopeEncodesNonScalarInputs(t *testing.T) {
	in := validEnvelopeInput()
	in.Inputs = map[string]any{"doc": map[string]any{"k": "v"}}
	got, err := BuildEnvelope(in)
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	if !strings.Contains(got, `"k":"v"`) {
		t.Fatalf("expected JSON-encoded map input, got:\n%s", got)
	}
}
