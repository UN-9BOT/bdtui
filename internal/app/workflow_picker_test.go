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
	id1, _ := m.projectIDForBeadsDir()
	if id1 == "" {
		t.Fatal("project id is empty")
	}
	if !validProjectID(id1) {
		t.Fatalf("project id %q is not a valid uuid hex", id1)
	}
	if id2, _ := m.projectIDForBeadsDir(); id2 != id1 {
		t.Fatalf("project id not stable: %q vs %q", id1, id2)
	}
	// The id must be persisted to disk and survive a "restart".
	path := filepath.Join(dir, projectIDFilename)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("persisted id file missing: %v", err)
	}
}

func TestProjectIDForBeadsDirDistinguishesWorkspaces(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	idA, _ := model{BeadsDir: dirA}.projectIDForBeadsDir()
	idB, _ := model{BeadsDir: dirB}.projectIDForBeadsDir()
	if idA == idB {
		t.Fatalf("distinct beads dirs must produce distinct project ids, both = %q", idA)
	}
}

func TestGenerateProjectIDIsUniqueAndValid(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		id, err := generateProjectID()
		if err != nil {
			t.Fatalf("generateProjectID: %v", err)
		}
		if !validProjectID(id) {
			t.Fatalf("invalid id: %q", id)
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id %q on iteration %d", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestValidProjectIDAcceptsHexVariants(t *testing.T) {
	for _, valid := range []string{
		"0123456789abcdef0123456789abcdef",
		"0123456789abcdef0123456789ABCDEF",
		"01234567-89ab-cdef-0123-456789abcdef",
	} {
		if !validProjectID(valid) {
			t.Fatalf("expected %q to be valid", valid)
		}
	}
	for _, bad := range []string{
		"",
		"not-hex",
		"0123456789abcdef0123456789abcde",  // 31 chars
		"0123456789abcdef0123456789abcdef0", // 33 chars
		"0123456789abcdef0123456789abcdeg",  // g is invalid
	} {
		if validProjectID(bad) {
			t.Fatalf("expected %q to be invalid", bad)
		}
	}
}

func TestProjectIDRegeneratesOnCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, projectIDFilename), []byte("not-a-uuid"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	m := model{BeadsDir: dir}
	// A corrupt file already on disk must surface as an error rather than
	// silently overwrite or return a fresh in-memory id: both would let
	// two concurrent launches disagree on the project scope.
	if _, err := m.projectIDForBeadsDir(); err == nil {
		t.Fatal("expected error for corrupt persisted id file")
	}
}

func TestProjectIDAtomicOnConcurrentFirstCall(t *testing.T) {
	// Two concurrent projectIDForBeadsDir() calls on a fresh dir must
	// agree on the same id (the O_CREATE|O_EXCL winner is read back by
	// the loser). N concurrent goroutines on the same empty dir produce
	// exactly the same project_id.
	dir := t.TempDir()
	m := model{BeadsDir: dir}
	const N = 16
	results := make([]string, N)
	errs := make([]error, N)
	done := make(chan struct{}, N)
	for i := 0; i < N; i++ {
		go func(i int) {
			results[i], errs[i] = m.projectIDForBeadsDir()
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < N; i++ {
		<-done
	}
	first := results[0]
	if errs[0] != nil {
		t.Fatalf("first call err: %v", errs[0])
	}
	for i := 1; i < N; i++ {
		if errs[i] != nil {
			t.Fatalf("call %d err: %v", i, errs[i])
		}
		if results[i] != first {
			t.Fatalf("call %d returned %q, want %q (winner)", i, results[i], first)
		}
	}
}
