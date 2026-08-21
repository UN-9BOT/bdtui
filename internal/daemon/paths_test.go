package daemon

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestPerProjectSocketIsolation verifies that two project IDs produce
// two distinct socket and DB paths. Without this, the TUI for project
// B would silently connect to the daemon for project A and the
// "single-active-Run per Beads task" invariant would be enforced in
// the wrong workspace.
func TestPerProjectSocketIsolation(t *testing.T) {
	const a, b = "project-a", "project-b"
	sockA := SocketPathForProject(a)
	sockB := SocketPathForProject(b)
	if sockA == sockB {
		t.Fatalf("SocketPathForProject collision: %s", sockA)
	}
	if !strings.Contains(sockA, a) {
		t.Errorf("socketA = %q, expected to contain %q", sockA, a)
	}
	if !strings.Contains(sockB, b) {
		t.Errorf("socketB = %q, expected to contain %q", sockB, b)
	}

	dbA := DBPathForProject(a)
	dbB := DBPathForProject(b)
	if dbA == dbB {
		t.Fatalf("DBPathForProject collision: %s", dbA)
	}
	dirA := filepath.Dir(sockA)
	dirB := filepath.Dir(sockB)
	if dirA == dirB {
		t.Errorf("socket parent dir collision: %s", dirA)
	}
	if dirA != filepath.Dir(dbA) {
		t.Errorf("socket dir != db dir for %q: %s vs %s", a, dirA, filepath.Dir(dbA))
	}
}

// TestEmptyProjectFallsBackToLegacy verifies that empty project_id
// resolves to the legacy global path so operator tooling that has
// not yet opted in to per-project routing keeps working.
func TestEmptyProjectFallsBackToLegacy(t *testing.T) {
	if SocketPathForProject("") != DefaultSocketPath() {
		t.Errorf("SocketPathForProject(\"\") != DefaultSocketPath()")
	}
	if DBPathForProject("") != DefaultDBPath() {
		t.Errorf("DBPathForProject(\"\") != DefaultDBPath()")
	}
}
