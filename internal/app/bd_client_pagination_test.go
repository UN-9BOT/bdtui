package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListIssuesUsesBoundedLimitAndParsesJSON(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	bdPath := filepath.Join(dir, "bd")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$ARGS_FILE"
printf '%s\n' '[{"id":"bd-1","title":"first","status":"open","priority":1,"issue_type":"task"},{"id":"bd-2","title":"second","status":"closed","priority":2,"issue_type":"bug"}]'
`
	if err := os.WriteFile(bdPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake bd: %v", err)
	}
	t.Setenv("ARGS_FILE", argsPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	issues, hash, err := NewBdClient(dir).ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if hash == "" || len(issues) != 2 || issues[0].ID != "bd-1" || issues[1].ID != "bd-2" {
		t.Fatalf("unexpected parsed issues: %#v, hash=%q", issues, hash)
	}

	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake bd args: %v", err)
	}
	got := strings.Fields(string(args))
	want := []string{"list", "--json", "--all", "--limit", "200"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("bd arguments = %q, want %q", got, want)
	}
}
