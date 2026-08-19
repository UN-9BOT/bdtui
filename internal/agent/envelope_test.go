package agent

import (
	"strings"
	"testing"

	"bdtui/internal/workflow"
)

const testDataSchema = `{"type":"object","required":["plan"],"properties":{"plan":{"type":"string"}}}`

func resolvedContract(t *testing.T, role workflow.RoleContract) ResultContract {
	t.Helper()
	c, err := ResolveContract(role, testDataSchema)
	if err != nil {
		t.Fatalf("ResolveContract: %v", err)
	}
	return c
}

func validRole() workflow.RoleContract {
	return workflow.RoleContract{
		ID:           "planner",
		Description:  "Plans the work",
		Workspace:    workflow.WorkspaceWrite,
		Outcomes:     []string{"planned", "needs_clarification"},
		Outputs:      []string{"plan", "alternatives"},
		ResultSchema: "schemas/result.json",
	}
}

func validEnvelopeInput() EnvelopeInput {
	role := validRole()
	c, err := ResolveContract(role, testDataSchema)
	if err != nil {
		panic(err) // testDataSchema and validRole are constructed; should not error.
	}
	return EnvelopeInput{
		Role:       role,
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
				"plan":         "/run/storage/plan.md",
				"alternatives": "/run/storage/alternatives.md",
			},
		},
		Contract: c,
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
		{"no outcomes", func(in *EnvelopeInput) {
			in.Contract.allowedOutcomes = nil
		}},
		{"no schema", func(in *EnvelopeInput) {
			in.Contract = ResultContract{allowedOutcomes: []string{"ok"}, declaredOutputs: []string{"plan"}}
		}},
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

// TestResolveContractEqualsRole asserts that ResolveContract makes
// DeclaredOutputs and AllowedOutcomes agree with the role — the bypass the
// reviewer flagged ("Role.Outputs=[plan], Contract.DeclaredOutputs=[]") is
// structurally impossible because the factory derives both from the role.
func TestResolveContractEqualsRole(t *testing.T) {
	role := validRole()
	c := resolvedContract(t, role)
	if err := c.consistentWith(role); err != nil {
		t.Fatalf("fresh contract must agree with role: %v", err)
	}
}

// TestResolveContractRejectsMismatch is the reviewer's regression test: a
// manually-constructed ResultContract that disagrees with the role (here
// DeclaredOutputs=[] against role.Outputs=["plan"]) is rejected by
// consistentWith. External callers cannot reach this state because the
// fields are unexported; this test proves the defense-in-depth check.
func TestResolveContractRejectsMismatch(t *testing.T) {
	role := validRole()
	bypass := ResultContract{
		schema:          testDataSchema,
		allowedOutcomes: []string{"planned"},
		declaredOutputs: nil, // bypass: empty declared outputs for role.Outputs=[plan]
	}
	if err := bypass.consistentWith(role); err == nil {
		t.Fatal("expected mismatch error for empty DeclaredOutputs vs role.Outputs=[plan]")
	}
	bypass2 := ResultContract{
		schema:          testDataSchema,
		allowedOutcomes: nil, // bypass: empty allowed outcomes
		declaredOutputs: []string{"plan"},
	}
	if err := bypass2.consistentWith(role); err == nil {
		t.Fatal("expected mismatch error for empty AllowedOutcomes vs role.Outcomes")
	}
}