package app

import (
	"os"
	"os/exec"
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
	repo, beads := mustGitRepoWithBeads(t)
	m := model{RepoDir: repo, BeadsDir: beads}
	id1, err := m.projectIDForBeadsDir()
	if err != nil {
		t.Fatalf("first projectIDForBeadsDir: %v", err)
	}
	if id1 == "" {
		t.Fatal("project id is empty")
	}
	if !validProjectID(id1) {
		t.Fatalf("project id %q is not a valid uuid hex", id1)
	}
	id2, err := m.projectIDForBeadsDir()
	if err != nil {
		t.Fatalf("second projectIDForBeadsDir: %v", err)
	}
	if id2 != id1 {
		t.Fatalf("project id not stable: %q vs %q", id1, id2)
	}
	// The id must be persisted to git config and survive a "restart" (a
	// fresh model that just reads the same repo).
	id3, err := model{RepoDir: repo, BeadsDir: beads}.projectIDForBeadsDir()
	if err != nil {
		t.Fatalf("restart projectIDForBeadsDir: %v", err)
	}
	if id3 != id1 {
		t.Fatalf("restart project id drift: %q vs %q", id1, id3)
	}
	// And it must NOT have leaked into the worktree as a tracked or
	// untracked file.
	cmd := exec.Command("git", "-C", repo, "ls-files", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	if strings.Contains(string(out), "bdtui-project-id") {
		t.Fatalf("project id leaked into worktree: %s", out)
	}
}

func TestProjectIDForBeadsDirDistinguishesWorkspaces(t *testing.T) {
	repoA, beadsA := mustGitRepoWithBeads(t)
	repoB, beadsB := mustGitRepoWithBeads(t)
	idA, errA := model{RepoDir: repoA, BeadsDir: beadsA}.projectIDForBeadsDir()
	if errA != nil {
		t.Fatalf("workspace A: %v", errA)
	}
	idB, errB := model{RepoDir: repoB, BeadsDir: beadsB}.projectIDForBeadsDir()
	if errB != nil {
		t.Fatalf("workspace B: %v", errB)
	}
	if idA == idB {
		t.Fatalf("distinct repos must produce distinct project ids, both = %q", idA)
	}
}

// mustGitRepoWithBeads returns a (repoDir, beadsDir) pair where repoDir is
// a fresh `git init` directory and beadsDir is repoDir/.beads. project id
// tests need a real git workspace because the id lives in git config.
func mustGitRepoWithBeads(t *testing.T) (repoDir, beadsDir string) {
	t.Helper()
	repoDir = t.TempDir()
	beadsDir = filepath.Join(repoDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o700); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	cmd := exec.Command("git", "-C", repoDir, "init", "-q", "--initial-branch=main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	// git refuses to write local config until identity is configured, which
	// would break our `git config --local bdtui.project-id ...` writes.
	cmd = exec.Command("git", "-C", repoDir, "config", "user.email", "bdtui-test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.email: %v: %s", err, out)
	}
	cmd = exec.Command("git", "-C", repoDir, "config", "user.name", "bdtui-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.name: %v: %s", err, out)
	}
	return repoDir, beadsDir
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
		"0123456789abcdef0123456789abcde",   // 31 chars
		"0123456789abcdef0123456789abcdef0", // 33 chars
		"0123456789abcdef0123456789abcdeg",  // g is invalid
	} {
		if validProjectID(bad) {
			t.Fatalf("expected %q to be invalid", bad)
		}
	}
}

func TestProjectIDAtomicOnConcurrentFirstCall(t *testing.T) {
	// Two concurrent projectIDForBeadsDir() calls on a fresh git repo
	// must agree on the same id. git's lock on .git/config.lock
	// serializes the writes; whichever writer lands last owns the
	// canonical value, and every other goroutine re-reads that value.
	repo, beads := mustGitRepoWithBeads(t)
	m := model{RepoDir: repo, BeadsDir: beads}
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
	if first == "" {
		t.Fatal("first call returned empty id")
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
