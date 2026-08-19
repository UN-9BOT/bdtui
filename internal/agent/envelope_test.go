package agent

import (
	"strings"
	"testing"

	"bdtui/internal/workflow"
)

func validEnvelopeInput() EnvelopeInput {
	return EnvelopeInput{
		Role: workflow.RoleContract{
			ID:           "planner",
			Description:  "Plans the work",
			Workspace:    workflow.WorkspaceWrite,
			Outcomes:     []string{"planned", "needs_clarification"},
			Outputs:      []string{"plan", "alternatives"},
			ResultSchema: `{"type":"object","required":["plan"],"properties":{"plan":{"type":"string"}}}`,
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
			Result: "/run/storage/result.json",
			Artifacts: map[string]string{
				"plan":        "/run/storage/plan.md",
				"alternatives": "/run/storage/alternatives.md",
			},
		},
		Contract: ResultContract{
			Schema:          `{"type":"object","required":["plan"],"properties":{"plan":{"type":"string"}}}`,
			AllowedOutcomes: []string{"planned", "needs_clarification"},
			DeclaredOutputs: []string{"plan", "alternatives"},
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
		t.Fatal("BuildEnvelope is not deterministic")
	}
}

func TestBuildEnvelopeSortsInputs(t *testing.T) {
	in1 := validEnvelopeInput()
	in1.Inputs = map[string]any{"a": 1, "b": 2}
	in2 := validEnvelopeInput()
	in2.Inputs = map[string]any{"b": 2, "a": 1}
	x, _ := BuildEnvelope(in1)
	y, _ := BuildEnvelope(in2)
	if x != y {
		t.Fatal("BuildEnvelope must render inputs in sorted-key order")
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
		{"declared output missing path", func(in *EnvelopeInput) {
			delete(in.OutputPaths.Artifacts, "alternatives")
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

func TestBuildEnvelopeContainsContract(t *testing.T) {
	got, err := BuildEnvelope(validEnvelopeInput())
	if err != nil {
		t.Fatalf("BuildEnvelope: %v", err)
	}
	for _, needle := range []string{
		"/run/storage/result.json",
		"/run/storage/plan.md",
		"/run/storage/alternatives.md",
		"declared_outputs:",
		"allowed_outcomes:",
		"# Output Contract",
		"\"outcome\"",
		"\"data\"",
	} {
		if !strings.Contains(got, needle) {
			t.Fatalf("envelope missing %q\n---\n%s", needle, got)
		}
	}
}