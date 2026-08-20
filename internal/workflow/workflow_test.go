package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const validWorkflow = `
version: 1
name: ship
steps:
  - id: plan
    type: agent
    role: planner
    on:
      planned: review_initial
  - id: review_initial
    type: agent
    role: reviewer
    inputs:
      plan:
        step: plan
        output: plan
    on:
      approved: implement_initial
      revise: revise_plan
      question: ask
  - id: implement_initial
    type: agent
    role: implementer
    inputs:
      plan:
        step: plan
        output: plan
      review:
        step: review_initial
        output: review
    on:
      done: end
  - id: ask
    type: human
    prompt: "Please clarify"
    on:
      answered: replan
  - id: replan
    type: agent
    role: planner
    inputs:
      clarification:
        step: ask
        output: response
    on:
      planned: review_replan
  - id: review_replan
    type: agent
    role: reviewer
    inputs:
      plan:
        step: replan
        output: plan
    on:
      approved: implement_replan
      revise: end
      question: ask
  - id: implement_replan
    type: agent
    role: implementer
    inputs:
      plan:
        step: replan
        output: plan
      review:
        step: review_replan
        output: review
    on:
      done: end
  - id: revise_plan
    type: agent
    role: planner
    inputs:
      review:
        step: review_initial
        output: review
    on:
      planned: review_revised
  - id: review_revised
    type: agent
    role: reviewer
    inputs:
      plan:
        step: revise_plan
        output: plan
    on:
      approved: implement_revised
      revise: end
      question: end
  - id: implement_revised
    type: agent
    role: implementer
    inputs:
      plan:
        step: revise_plan
        output: plan
      review:
        step: review_revised
        output: review
    on:
      done: end
  - id: end
    type: end
`

const plannerRole = `
id: planner
prompt: prompts/planner.md
outcomes: [planned]
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
	if spec.Version != 1 || spec.Name != "ship" || len(spec.Steps) != 11 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
	if spec.Steps[0].Role != "planner" || spec.Steps[3].Type != StepHuman || spec.Steps[10].Type != StepEnd {
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

func TestParseRejectsYAMLAnchor(t *testing.T) {
	_, err := Parse([]byte("version: 1\nname: &n ship\nsteps: []\n"))
	if err == nil || !strings.Contains(err.Error(), "anchor") {
		t.Fatalf("expected anchor rejection, got %v", err)
	}
}

func TestParseRejectsYAMLAlias(t *testing.T) {
	// A valid alias necessarily references a preceding anchor, so the
	// preflight reports the anchor first; the alias branch itself is covered
	// in TestValidateYAMLNodeRejectsAlias.
	_, err := Parse([]byte("base: &b {type: agent}\nuse: *b\n"))
	if err == nil {
		t.Fatal("expected alias-bearing document to be rejected")
	}
}

func TestValidateYAMLNodeRejectsAlias(t *testing.T) {
	err := validateYAMLNode(&yaml.Node{Kind: yaml.AliasNode, Value: "b"})
	if err == nil || !strings.Contains(err.Error(), "alias") {
		t.Fatalf("expected alias rejection, got %v", err)
	}
}

func TestParseRejectsYAMLMergeKey(t *testing.T) {
	_, err := Parse([]byte("version: 1\nname: ship\nsteps: []\nextras: {<<: {p: 1}}\n"))
	if err == nil || !strings.Contains(err.Error(), "merge key") {
		t.Fatalf("expected merge key rejection, got %v", err)
	}
}

func TestParseRejectsYAMLCustomTag(t *testing.T) {
	_, err := Parse([]byte("version: 1\nname: ship\nsteps: []\nfoo: !custom value\n"))
	if err == nil || !strings.Contains(err.Error(), "custom tag") {
		t.Fatalf("expected custom tag rejection, got %v", err)
	}
}

func TestParseRejectsLongFormCustomTag(t *testing.T) {
	_, err := Parse([]byte("version: 1\nname: !<tag:yaml.org,2002:evil> ship\nsteps: []\n"))
	if err == nil || !strings.Contains(err.Error(), "custom tag") {
		t.Fatalf("expected long-form custom tag rejection, got %v", err)
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
		{"agent missing role", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    on: {go: b}\n  - id: b\n    type: end\n", "agent step role"},
		{"role path traversal", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: ../../evil\n    on: {go: b}\n  - id: b\n    type: end\n", "path separators"},
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
		{"source does not dominate", "version: 1\nname: x\nsteps:\n  - id: a\n    type: agent\n    role: r\n    inputs:\n      x: {step: b, output: y}\n    on: {go: b}\n  - id: b\n    type: agent\n    role: r\n    on: {go: a}\n", "does not dominate"},
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

func completeFiles() map[string]string {
	return map[string]string{
		"roles/planner/prompt":     "planner prompt",
		"roles/planner/schema":     `{"type":"object"}`,
		"roles/reviewer/prompt":    "reviewer prompt",
		"roles/reviewer/schema":    `{"type":"object"}`,
		"roles/implementer/prompt": "implementer prompt",
		"roles/implementer/schema": `{"type":"object"}`,
	}
}

func TestBundleValidate(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	bundle := Bundle{
		Spec:           *spec,
		Roles:          validRoles(),
		Files:          completeFiles(),
		WorkflowSource: validWorkflow,
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("bundle validate: %v", err)
	}
}

func TestBundleValidateMissingDependencyFile(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	files := completeFiles()
	delete(files, "roles/reviewer/prompt")

	bundle := Bundle{Spec: *spec, Roles: validRoles(), Files: files, WorkflowSource: validWorkflow}
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "missing prompt dependency") {
		t.Fatalf("validate = %v, want missing-dependency error", err)
	}
}

func TestBundleValidateOutcomeNotAllowed(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Add an outcome to review_initial that the reviewer role does not allow,
	// while keeping every declared reviewer outcome covered.
	steps := spec.Steps
	steps[1].On = map[string]string{
		"approved": "implement_initial",
		"revise":   "revise_plan",
		"question": "ask",
		"bogus":    "end",
	}
	spec.Steps = steps

	bundle := Bundle{Spec: *spec, Roles: validRoles(), Files: completeFiles(), WorkflowSource: validWorkflow}
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "not allowed by role") {
		t.Fatalf("validate = %v, want outcome-not-allowed error", err)
	}
}

func TestBundleValidateMissingTransition(t *testing.T) {
	spec := &WorkflowSpec{
		Version: 1,
		Name:    "m",
		Steps: []StepSpec{
			{ID: "a", Type: StepAgent, Role: "r", On: map[string]string{"x": "end"}},
			{ID: "end", Type: StepEnd},
		},
	}
	roles := map[string]RoleContract{
		"r": {
			ID:           "r",
			Prompt:       "p.md",
			Outcomes:     []string{"x", "y"},
			Outputs:      []string{"o"},
			ResultSchema: "s.json",
			Workspace:    WorkspaceRead,
		},
	}
	files := map[string]string{"roles/r/prompt": "p", "roles/r/schema": "s"}
	bundle := Bundle{Spec: *spec, Roles: roles, Files: files, WorkflowSource: "version: 1\nname: m\nsteps:\n  - id: a\n    type: agent\n    role: r\n    on: {x: end}\n  - id: end\n    type: end\n"}

	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "has no transition") {
		t.Fatalf("validate = %v, want missing-transition error", err)
	}
}

func TestBundleValidateUndeclaredInputOutput(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// The review step consumes an output the planner role does not declare.
	steps := spec.Steps
	steps[1].Inputs = map[string]InputRef{"plan": {Step: "plan", Output: "nonexistent"}}
	spec.Steps = steps

	bundle := Bundle{Spec: *spec, Roles: validRoles(), Files: completeFiles(), WorkflowSource: validWorkflow}
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "not produced by step") {
		t.Fatalf("validate = %v, want undeclared-output error", err)
	}
}

func TestBundleValidateHumanResponseOutput(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// validWorkflow flows ask.response -> replan.clarification.
	bundle := Bundle{Spec: *spec, Roles: validRoles(), Files: completeFiles(), WorkflowSource: validWorkflow}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("human response dataflow: %v", err)
	}

	badSpec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	badSpec.Steps[4].Inputs = map[string]InputRef{"clarification": {Step: "ask", Output: "wrong"}}
	bad := Bundle{Spec: *badSpec, Roles: validRoles(), Files: completeFiles(), WorkflowSource: validWorkflow}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "not produced by step") {
		t.Fatalf("validate = %v, want not-produced-by-step error", err)
	}
}

func TestBuildSnapshotDeterministicAndSensitive(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	bundle := Bundle{
		Spec:           *spec,
		Roles:          validRoles(),
		Files:          completeFiles(),
		WorkflowSource: validWorkflow,
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
	if !strings.Contains(s1.JSON, "workflow_source") {
		t.Fatal("snapshot should contain workflow_source")
	}

	changed := bundle
	changed.Files = map[string]string{
		"roles/planner/prompt":     "planner prompt changed",
		"roles/planner/schema":     `{"type":"object"}`,
		"roles/reviewer/prompt":    "reviewer prompt",
		"roles/reviewer/schema":    `{"type":"object"}`,
		"roles/implementer/prompt": "implementer prompt",
		"roles/implementer/schema": `{"type":"object"}`,
	}
	s3, err := BuildSnapshot(changed)
	if err != nil {
		t.Fatalf("snapshot 3: %v", err)
	}
	if s3.Ref == s1.Ref {
		t.Fatal("snapshot ref should change when a dependency file changes")
	}

	changedSource := bundle
	changedSource.WorkflowSource = validWorkflow + "\n"
	s4, err := BuildSnapshot(changedSource)
	if err != nil {
		t.Fatalf("snapshot 4: %v", err)
	}
	if s4.Ref == s1.Ref {
		t.Fatal("snapshot ref should change when workflow_source changes")
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
	if bundle.Files["roles/planner/prompt"] != "project planner prompt" {
		t.Fatalf("missing project planner prompt: %+v", bundle.Files)
	}
	if bundle.Files["roles/reviewer/prompt"] != "reviewer prompt" {
		t.Fatalf("missing global reviewer prompt: %+v", bundle.Files)
	}
}

func TestLoaderRoleIDMismatch(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global")

	mustWriteDir(t, global, "workflows/wf.yaml", validWorkflow)
	mustWriteDir(t, global, "roles/planner.yaml", strings.Replace(plannerRole, "id: planner", "id: implementer", 1))
	mustWriteDir(t, global, "roles/reviewer.yaml", reviewerRole)
	mustWriteDir(t, global, "roles/implementer.yaml", implementerRole)
	mustWriteDir(t, global, "prompts/planner.md", "planner")
	mustWriteDir(t, global, "prompts/reviewer.md", "reviewer")
	mustWriteDir(t, global, "prompts/implementer.md", "implementer")
	mustWriteDir(t, global, "schemas/plan.json", `{}`)
	mustWriteDir(t, global, "schemas/review.json", `{}`)
	mustWriteDir(t, global, "schemas/patch.json", `{}`)

	loader := Loader{Global: global}
	if _, err := loader.Load(context.Background(), "wf"); err == nil || !strings.Contains(err.Error(), "declares id") {
		t.Fatalf("load = %v, want role-id-mismatch error", err)
	}
}

func TestLoaderNamespacesDependencyFiles(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global")
	project := filepath.Join(dir, "project")

	const workflowYAML = `
version: 1
name: collision
steps:
  - id: a
    type: agent
    role: ra
    on: {done: b}
  - id: b
    type: agent
    role: rb
    on: {done: end}
  - id: end
    type: end
`
	const raRole = `
id: ra
prompt: prompts/ra.md
outcomes: [done]
outputs: [out]
result_schema: schemas/result.json
workspace: read
`
	const rbRole = `
id: rb
prompt: prompts/rb.md
outcomes: [done]
outputs: [out]
result_schema: schemas/result.json
workspace: read
`

	mustWriteDir(t, global, "workflows/collision.yaml", workflowYAML)
	mustWriteDir(t, global, "roles/rb.yaml", rbRole)
	mustWriteDir(t, global, "prompts/rb.md", "global rb prompt")
	mustWriteDir(t, global, "schemas/result.json", "global-schema")

	mustWriteDir(t, project, "roles/ra.yaml", raRole)
	mustWriteDir(t, project, "prompts/ra.md", "project ra prompt")
	mustWriteDir(t, project, "schemas/result.json", "project-schema")

	loader := Loader{Global: global, Project: project}
	bundle, err := loader.Load(context.Background(), "collision")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if bundle.Files["roles/ra/schema"] != "project-schema" {
		t.Fatalf("ra schema = %q, want project-schema", bundle.Files["roles/ra/schema"])
	}
	if bundle.Files["roles/rb/schema"] != "global-schema" {
		t.Fatalf("rb schema = %q, want global-schema", bundle.Files["roles/rb/schema"])
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

func TestLoaderListEmpty(t *testing.T) {
	loader := Loader{}
	got, err := loader.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty list = %d, want 0", len(got))
	}
}

func TestLoaderListFromBothRoots(t *testing.T) {
	global := t.TempDir()
	project := t.TempDir()

	mustWriteDir(t, global, "workflows/alpha.yaml", validWorkflow)
	mustWriteDir(t, global, "workflows/beta.yaml", validWorkflow)
	mustWriteDir(t, project, "workflows/gamma.yaml", validWorkflow)
	mustWriteDir(t, global, "workflows/notyaml.txt", "ignored")

	loader := Loader{Global: global, Project: project}
	got, err := loader.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []ListEntry{
		{Name: "alpha", Origin: "global"},
		{Name: "beta", Origin: "global"},
		{Name: "gamma", Origin: "project"},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoaderListProjectWinsOnCollision(t *testing.T) {
	global := t.TempDir()
	project := t.TempDir()

	mustWriteDir(t, global, "workflows/shared.yaml", validWorkflow)
	mustWriteDir(t, global, "workflows/globalonly.yaml", validWorkflow)
	mustWriteDir(t, project, "workflows/shared.yaml", validWorkflow)
	mustWriteDir(t, project, "workflows/projectonly.yaml", validWorkflow)

	loader := Loader{Global: global, Project: project}
	got, err := loader.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	want := map[string]string{"globalonly": "global", "projectonly": "project", "shared": "project"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for _, e := range got {
		if w, ok := want[e.Name]; !ok || w != e.Origin {
			t.Fatalf("entry %+v not in want %v", e, want)
		}
	}
}
