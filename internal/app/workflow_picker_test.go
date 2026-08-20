package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectWorkflowsRootIsLayoutRoot(t *testing.T) {
	dir := t.TempDir()
	m := model{BeadsDir: dir}
	root := m.projectWorkflowsRoot()
	if root != dir {
		t.Fatalf("project root = %q, want %q (no extra /workflows)", root, dir)
	}

	// Loader contract: project root + "/workflows/<name>.yaml" must resolve
	// to the actual file. This is the regression test for the double-path
	// bug where the TUI used to append /workflows itself, causing the loader
	// to look in <dir>/workflows/workflows.
	mustMkdir(t, filepath.Join(dir, "workflows"))
	mustWrite(t, filepath.Join(dir, "workflows", "ship.yaml"), validPickerWorkflow)

	// And the picker option list must surface the project workflow. We test
	// List() (not Load) because Load validates the full bundle, which is
	// orthogonal to path resolution.
	opts, err := m.loadWorkflowOptions()
	if err != nil {
		t.Fatalf("loadWorkflowOptions: %v", err)
	}
	found := false
	for _, o := range opts {
		if o.Name == "ship" && o.Origin == "project" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected project workflow 'ship', got %v", opts)
	}

	// Negative case: a yaml in <dir>/workflows/workflows must NOT be
	// surfaced (it is at the wrong depth after the fix).
	mustMkdir(t, filepath.Join(dir, "workflows", "workflows"))
	mustWrite(t, filepath.Join(dir, "workflows", "workflows", "ghost.yaml"), validPickerWorkflow)
	opts2, err := m.loadWorkflowOptions()
	if err != nil {
		t.Fatalf("loadWorkflowOptions (2): %v", err)
	}
	for _, o := range opts2 {
		if o.Name == "ghost" {
			t.Fatalf("workflow at <dir>/workflows/workflows should be invisible, got %+v", o)
		}
	}
}

func TestDefaultGlobalWorkflowsRootIsLayoutRoot(t *testing.T) {
	// Sanity guard: the constant must NOT include "/workflows" or the loader
	// would scan <const>/workflows/workflows. Regression guard for the same
	// double-path bug at the global side.
	if strings.Contains(defaultGlobalWorkflowsRoot, "/workflows") {
		t.Fatalf("global root %q must not contain /workflows", defaultGlobalWorkflowsRoot)
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const validPickerWorkflow = `
version: 1
name: ship
steps:
  - id: plan
    type: agent
    role: planner
    on:
      planned: end
`

func TestProjectIDForBeadsDirIsStable(t *testing.T) {
	dir := t.TempDir()
	m := model{BeadsDir: dir}
	id1 := m.projectIDForBeadsDir()
	if id1 == "" {
		t.Fatal("project id is empty")
	}
	if len(id1) != 16 {
		t.Fatalf("project id len = %d, want 16 hex chars", len(id1))
	}
	if id2 := m.projectIDForBeadsDir(); id2 != id1 {
		t.Fatalf("project id not stable: %q vs %q", id1, id2)
	}
}

func TestProjectIDForBeadsDirDistinguishesWorkspaces(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	idA := model{BeadsDir: dirA}.projectIDForBeadsDir()
	idB := model{BeadsDir: dirB}.projectIDForBeadsDir()
	if idA == idB {
		t.Fatalf("distinct beads dirs must produce distinct project ids, both = %q", idA)
	}
}
