package bdtui_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBdClientListIssuesParsesDeferUntil is an end-to-end check that the real
// `bd` binary's --defer flag is reflected in the parsed Issue.
func TestBdClientListIssuesParsesDeferUntil(t *testing.T) {
	t.Parallel()

	bdPath, err := exec.LookPath("bd")
	if err != nil {
		t.Skip("bd binary not available in PATH")
	}

	tmp := t.TempDir()
	runIn := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bdPath, args...)
		cmd.Dir = tmp
		cmd.Env = append(os.Environ(), "HOME="+tmp)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd %s failed: %v | %s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}

	beadsDir := filepath.Join(tmp, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	runIn("init", "--prefix", "dt")
	runIn("create", "smoke deferred", "--defer", "2099-12-30", "--silent")

	client := NewBdClient(tmp)
	issues, _, err := client.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	if len(issues) == 0 {
		t.Fatalf("expected at least one issue from bd list")
	}
	if issues[0].Title != "smoke deferred" {
		t.Fatalf("unexpected first issue title: %q", issues[0].Title)
	}
	if !issues[0].IsDeferred() {
		t.Fatalf("expected IsDeferred()=true for first issue, got DeferUntil=%q", issues[0].DeferUntil)
	}
	if issues[0].DeferUntil == "" {
		t.Fatalf("expected DeferUntil to be populated, got empty")
	}
}
