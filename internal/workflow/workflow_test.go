package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validWorkflow = `
name: ship
steps:
  - id: write
    type: agent
    role: roles/writer.md
    inputs: [task]
    outputs: [patch]
    result_schema: schemas/result.json
    next: review
  - id: review
    type: agent
    role: roles/reviewer.md
    inputs: [patch]
    outputs: [review]
    next: approve
  - id: approve
    type: human
    prompt: "Approve the change?"
    next: done
  - id: done
    type: end
`

func TestParseValid(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if spec.Name != "ship" || len(spec.Steps) != 4 {
		t.Fatalf("unexpected spec: %+v", spec)
	}
	if spec.Steps[0].Type != StepAgent || spec.Steps[3].Type != StepEnd {
		t.Fatalf("unexpected step types: %+v", spec.Steps)
	}
}

func TestParseUnknownField(t *testing.T) {
	_, err := Parse([]byte(`
name: ship
steps:
  - id: write
    type: agent
    role: roles/writer.md
    next: done
  - id: done
    type: end
bogus: true
`))
	if err == nil {
		t.Fatal("expected unknown-field error")
	}
}

func TestParseUnknownStepField(t *testing.T) {
	_, err := Parse([]byte(`
name: ship
steps:
  - id: write
    type: agent
    role: roles/writer.md
    next: done
    wat: 1
  - id: done
    type: end
`))
	if err == nil {
		t.Fatal("expected unknown step field error")
	}
}

func TestParseMultipleDocuments(t *testing.T) {
	_, err := Parse([]byte(validWorkflow + "\n---\nname: second\nsteps: []\n"))
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
		{"empty name", "steps:\n  - id: a\n    type: end\n", "name is required"},
		{"no steps", "name: x\nsteps: []\n", "at least one step"},
		{"duplicate id", "name: x\nsteps:\n  - id: a\n    type: agent\n    role: r.md\n    next: b\n  - id: a\n    type: end\n", "duplicate step id"},
		{"invalid type", "name: x\nsteps:\n  - id: a\n    type: wat\n    next: b\n  - id: b\n    type: end\n", "invalid type"},
		{"agent missing role", "name: x\nsteps:\n  - id: a\n    type: agent\n    next: b\n  - id: b\n    type: end\n", "agent step requires role"},
		{"agent sets prompt", "name: x\nsteps:\n  - id: a\n    type: agent\n    role: r.md\n    prompt: hi\n    next: b\n  - id: b\n    type: end\n", "agent step must not set prompt"},
		{"human missing prompt", "name: x\nsteps:\n  - id: a\n    type: human\n    next: b\n  - id: b\n    type: end\n", "human step requires prompt"},
		{"end has next", "name: x\nsteps:\n  - id: a\n    type: agent\n    role: r.md\n    next: b\n  - id: b\n    type: end\n    next: a\n", "end step must not have next"},
		{"next not found", "name: x\nsteps:\n  - id: a\n    type: agent\n    role: r.md\n    next: missing\n", "not found"},
		{"self next", "name: x\nsteps:\n  - id: a\n    type: agent\n    role: r.md\n    next: a\n", "must not be itself"},
		{"cycle", "name: x\nsteps:\n  - id: a\n    type: agent\n    role: r.md\n    next: b\n  - id: b\n    type: agent\n    role: r.md\n    next: a\n", "cycle detected"},
		{"unreachable", "name: x\nsteps:\n  - id: a\n    type: agent\n    role: r.md\n    next: b\n  - id: b\n    type: end\n  - id: orphan\n    type: end\n", "not reachable"},
		{"entry is end", "name: x\nsteps:\n  - id: a\n    type: end\n", "first step must not be end"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := Parse([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			err = spec.Validate()
			if err == nil {
				t.Fatalf("expected validation error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
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
	if strings.Contains(aj, "null") {
		t.Fatalf("canonical JSON should not contain null slices: %s", aj)
	}
}

func TestBuildSnapshotDeterministicAndSensitive(t *testing.T) {
	spec, err := Parse([]byte(validWorkflow))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	closure := Closure{Spec: *spec, Files: map[string]string{
		"roles/writer.md":     "writer prompt",
		"roles/reviewer.md":   "reviewer prompt",
		"schemas/result.json": `{"type":"object"}`,
	}}

	s1, err := BuildSnapshot(closure)
	if err != nil {
		t.Fatalf("snapshot 1: %v", err)
	}
	s2, err := BuildSnapshot(closure)
	if err != nil {
		t.Fatalf("snapshot 2: %v", err)
	}
	if s1.Ref != s2.Ref || s1.JSON != s2.JSON {
		t.Fatal("snapshot not deterministic")
	}
	if len(s1.Ref) != 64 {
		t.Fatalf("ref length = %d, want 64", len(s1.Ref))
	}

	changed := Closure{Spec: *spec, Files: map[string]string{
		"roles/writer.md":     "writer prompt",
		"roles/reviewer.md":   "reviewer prompt",
		"schemas/result.json": `{"type":"object","required":["ok"]}`,
	}}
	s3, err := BuildSnapshot(changed)
	if err != nil {
		t.Fatalf("snapshot 3: %v", err)
	}
	if s3.Ref == s1.Ref {
		t.Fatal("snapshot ref should change when a dependency file changes")
	}
}

func TestLoaderProjectOverrideAndGlobalFallback(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global")
	project := filepath.Join(dir, "project")
	globalWF := "name: global\nsteps:\n  - id: a\n    type: human\n    prompt: hi\n    next: b\n  - id: b\n    type: end\n"
	projectWF := "name: project\nsteps:\n  - id: a\n    type: human\n    prompt: hi\n    next: b\n  - id: b\n    type: end\n"
	mustWriteDir(t, global, "wf.yaml", globalWF)
	mustWriteDir(t, project, "wf.yaml", projectWF)
	mustWriteDir(t, global, "only-global.yaml", "name: onlyglobal\nsteps:\n  - id: a\n    type: human\n    prompt: hi\n    next: b\n  - id: b\n    type: end\n")

	loader := Loader{GlobalDir: global, ProjectDir: project}

	// project overrides global wholesale
	got, err := loader.Load(context.Background(), "wf")
	if err != nil {
		t.Fatalf("load wf: %v", err)
	}
	if got.Spec.Name != "project" {
		t.Fatalf("name = %q, want project", got.Spec.Name)
	}

	// global fallback when project does not define it
	got, err = loader.Load(context.Background(), "only-global")
	if err != nil {
		t.Fatalf("load only-global: %v", err)
	}
	if got.Spec.Name != "onlyglobal" {
		t.Fatalf("name = %q, want onlyglobal", got.Spec.Name)
	}

	if _, err := loader.Load(context.Background(), "missing"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestLoaderCollectsDependencies(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")

	wf := "name: ship\nsteps:\n  - id: write\n    type: agent\n    role: roles/writer.md\n    result_schema: schemas/result.json\n    next: done\n  - id: done\n    type: end\n"
	mustWriteDir(t, project, "ship.yaml", wf)
	mustWriteDir(t, project, "roles/writer.md", "writer prompt")
	mustWriteDir(t, project, "schemas/result.json", `{"type":"object"}`)

	loader := Loader{ProjectDir: project}
	got, err := loader.Load(context.Background(), "ship")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Files["roles/writer.md"] != "writer prompt" {
		t.Fatalf("missing role file: %+v", got.Files)
	}
	if got.Files["schemas/result.json"] != `{"type":"object"}` {
		t.Fatalf("missing schema file: %+v", got.Files)
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
