package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validWorkflow = `
version: 1
name: ship
steps:
  - id: plan
    type: agent
    role: planner
    on:
      planned: review
      question: ask
  - id: review
    type: agent
    role: reviewer
    inputs:
      plan:
        step: plan
        output: plan
    on:
      approved: implement
      revise: plan
      question: ask
  - id: ask
    type: human
    inputs:
      question:
        step: review
        output: question
    prompt: "Please clarify"
    on:
      answered: plan
  - id: implement
    type: agent
    role: implementer
    inputs:
      plan:
        step: plan
        output: plan
      review:
        step: review
        output: review
    on:
      done: done
  - id: done
    type: end
`

const plannerRole = `
id: planner
prompt: prompts/planner.md
outcomes: [planned, question]
outputs: [plan]
result_schema: schemas/plan.json
workspace: read
`

const reviewerRole = `
id: reviewer
prompt: prompts/reviewer.md
outcomes: [approved, revise, question]
outputs: [review]
result_schema: schemas/review.json
workspace: read
`

const implementerRole = `
id: implementer
prompt: prompts/implementer.md
outcomes: [done]
outputs: [patch]
result_schema: schemas/patch.json
workspace: write
`

func TestParseValid(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if spec.Version != 1 || spec.Name != "ship" || len(spec.Steps) != 5 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
	if spec.Steps[0].Role != "planner" || spec.Steps[2].Type != StepHuman || spec.Steps[4].Type != StepEnd {
		t.Fatalf("unexpected steps: %+v", spec.Steps)
	}
}

func TestParseUnknownField(t *testing.T) {
	_, err := Parse([]byte(validWorkflow + "\nbogus: true\n"))
	if err == nil {
		t.Fatal("expected unknown-field error")
	}
}

func TestParseUnknownStepField(t *testing.T) {
	_, err := Parse([]byte(`
version: 1
name: ship
steps:
  - id: plan
    type: agent
    role: planner
    on: { planned: done }
    wat: 1
  - id: done
    type: end
`))
	if err == nil {
		t.Fatal("expected unknown step field error")
	}
}

func TestParseMultipleDocuments(t *testing.T) {
	_, err := Parse([]byte(validWorkflow + "\n---\nversion: 1\nname: second\nsteps: []\n"))
	if err == nil {
		t.Fatal("expected multiple-document error")
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"missing version", "name: x\nsteps:\n  - id: a\n    type: end\n", "unsupported version"},
		{"empty name", "version: 1\nsteps:\n  - id: a\n    type: end\n", "name is required"},
		{"no steps", "version: 1\nname: x\nsteps: []\n", "at least one step"},
		{"duplicate id", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: r\n    on: {go: b}\n  - id: a\n    type: end\n", "duplicate step id"},
		{"invalid type", "version: 1\nname: x\nsteps:\n  - id: a\n    type: wat\n    on: {go: b}\n  - id: b\n    type: end\n", "invalid type"},
		{"agent missing role", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    on: {go: b}\n  - id: b\n    type: end\n", "agent step requires role"},
		{"agent sets prompt", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: r\n    prompt: hi\n    on: {go: b}\n  - id: b\n    type: end\n", "agent step must not set prompt"},
		{"agent missing on", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: r\n  - id: b\n    type: end\n", "at least one outcome"},
		{"human sets role", "version: 1\nname: x\nsteps:\n  - id: a\n    type: human\n    role: r\n    on: {go: b}\n  - id: b\n    type: end\n", "human step must not set role"},
		{"human missing on", "version: 1\nname: x\nsteps:\n  - id: a\n    type: human\n    prompt: hi\n  - id: b\n    type: end\n", "at least one outcome"},
		{"end has on", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: r\n    on: {go: b}\n  - id: b\n    type: end\n    on: {x: a}\n", "end step must not set"},
		{"on target not found", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: r\n    on: {go: missing}\n", "not found"},
		{"input step not found", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: r\n    inputs:\n      x: {step: missing, output: y}\n    on: {go: b}\n  - id: b\n    type: end\n", "not found"},
		{"input step is end", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: r\n    inputs:\n      x: {step: b, output: y}\n    on: {go: b}\n  - id: b\n    type: end\n", "end step"},
		{"input missing output", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: r\n    inputs:\n      x: {step: a, output: \"\"}\n    on: {go: b}\n  - id: b\n    type: end\n", "output is required"},
		{"unreachable", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: r\n    on: {go: b}\n  - id: b\n    type: end\n  - id: orphan\n    type: end\n", "not reachable"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := Parse([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			err = spec.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestCyclesAllowed(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// validWorkflow contains plan -> review -> revise -> plan and
	// review -> question -> ask -> answered -> plan cycles.
	if err := spec.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestCanonicalJSONDeterministic(t *testing.T) {
	a, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	aj, err := a.CanonicalJSON()
	if err != nil {
		t.Fatalf("canonical a: %v", err)
	}
	bj, err := b.CanonicalJSON()
	if err != nil {
		t.Fatalf("canonical b: %v", err)
	}
	if aj != bj {
		t.Fatalf("canonical JSON not deterministic:\n%s\n%s", aj, bj)
	}
}

func validRoles() map[string]RoleContract {
	planner, _ := ParseRole([]byte(plannerRole))
	reviewer, _ := ParseRole([]byte(reviewerRole))
	implementer, _ := ParseRole([]byte(implementerRole))
	return map[string]RoleContract{
		"planner":     *planner,
		"reviewer":    *reviewer,
		"implementer": *implementer,
	}
}

func TestBundleValidate(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	bundle := Bundle{
		Spec:  *spec,
		Roles: validRoles(),
		Files: map[string]string{
			"prompts/planner.md":     "planner prompt",
			"prompts/reviewer.md":    "reviewer prompt",
			"prompts/implementer.md": "implementer prompt",
			"schemas/plan.json":      `{"type":"object"}`,
			"schemas/review.json":    `{"type":"object"}`,
			"schemas/patch.json":     `{"type":"object"}`,
		},
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("bundle validate: %v", err)
	}
}

func TestBundleValidateOutcomeNotAllowed(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	roles := validRoles()
	// Mutate planner outcomes so the plan step's "planned" outcome is no longer
	// allowed.
	p := roles["planner"]
	p.Outcomes = []string{"question"}
	roles["planner"] = p

	bundle := Bundle{Spec: *spec, Roles: roles}
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "not allowed by role") {
		t.Fatalf("validate = %v, want outcome-not-allowed error", err)
	}
}

func TestBundleValidateUndeclaredInputOutput(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	roles := validRoles()
	// Remove planner's "plan" output; the review step consumes plan from plan.
	p := roles["planner"]
	p.Outputs = []string{}
	roles["planner"] = p

	bundle := Bundle{Spec: *spec, Roles: roles}
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "not produced by step") {
		t.Fatalf("validate = %v, want undeclared-output error", err)
	}
}

func TestBuildSnapshotDeterministicAndSensitive(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	bundle := Bundle{
		Spec:  *spec,
		Roles: validRoles(),
		Files: map[string]string{
			"prompts/planner.md": "planner prompt",
			"schemas/plan.json":  `{"type":"object"}`,
		},
	}
	s1, err := BuildSnapshot(bundle)
	if err != nil {
		t.Fatalf("snapshot 1: %v", err)
	}
	s2, err := BuildSnapshot(bundle)
	if err != nil {
		t.Fatalf("snapshot 2: %v", err)
	}
	if s1.Ref != s2.Ref || s1.JSON != s2.JSON {
		t.Fatal("snapshot not deterministic")
	}
	if len(s1.Ref) != 64 {
		t.Fatalf("ref length = %d, want 64", len(s1.Ref))
	}

	changed := bundle
	changed.Files = map[string]string{
		"prompts/planner.md": "planner prompt changed",
		"schemas/plan.json":  `{"type":"object"}`,
	}
	s3, err := BuildSnapshot(changed)
	if err != nil {
		t.Fatalf("snapshot 3: %v", err)
	}
	if s3.Ref == s1.Ref {
		t.Fatal("snapshot ref should change when a dependency file changes")
	}
}

func TestLoaderWorkflowAndRoleOverride(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global")
	project := filepath.Join(dir, "project")

	// Global definitions.
	mustWriteDir(t, global, "workflows/wf.yaml", validWorkflow)
	mustWriteDir(t, global, "roles/planner.yaml", plannerRole)
	mustWriteDir(t, global, "roles/reviewer.yaml", reviewerRole)
	mustWriteDir(t, global, "roles/implementer.yaml", implementerRole)
	mustWriteDir(t, global, "prompts/planner.md", "global planner prompt")
	mustWriteDir(t, global, "prompts/reviewer.md", "reviewer prompt")
	mustWriteDir(t, global, "prompts/implementer.md", "implementer prompt")
	mustWriteDir(t, global, "schemas/plan.json", `{"type":"object"}`)
	mustWriteDir(t, global, "schemas/review.json", `{"type":"object"}`)
	mustWriteDir(t, global, "schemas/patch.json", `{"type":"object"}`)

	// Project overrides the planner role (whole-definition), but the workflow
	// and other roles come from global.
	projectPlanner := strings.Replace(plannerRole, "prompts/planner.md", "prompts/project-planner.md", 1)
	mustWriteDir(t, project, "roles/planner.yaml", projectPlanner)
	mustWriteDir(t, project, "prompts/project-planner.md", "project planner prompt")
	mustWriteDir(t, project, "schemas/plan.json", `{"type":"object"}`)

	loader := Loader{Global: global, Project: project}
	bundle, err := loader.Load(context.Background(), "wf")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if bundle.Spec.Name != "ship" {
		t.Fatalf("spec name = %q, want ship", bundle.Spec.Name)
	}
	if bundle.Roles["planner"].Prompt != "prompts/project-planner.md" {
		t.Fatalf("planner prompt = %q, want project override", bundle.Roles["planner"].Prompt)
	}
	if bundle.Roles["reviewer"].Prompt != "prompts/reviewer.md" {
		t.Fatalf("reviewer prompt = %q, want global fallback", bundle.Roles["reviewer"].Prompt)
	}
	if bundle.Files["prompts/project-planner.md"] != "project planner prompt" {
		t.Fatalf("missing project planner prompt: %+v", bundle.Files)
	}
	if bundle.Files["prompts/reviewer.md"] != "reviewer prompt" {
		t.Fatalf("missing global reviewer prompt: %+v", bundle.Files)
	}
}

func mustWriteDir(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}
