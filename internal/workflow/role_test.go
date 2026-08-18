package workflow

import (
	"strings"
	"testing"
)

const validRole = `
id: planner
description: Plans the implementation
prompt: prompts/planner.md
outcomes: [planned, question]
outputs: [plan]
result_schema: schemas/plan.json
workspace: read
`

func TestParseRoleValid(t *testing.T) {
	r, err := ParseRole([]byte(validRole))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if r.ID != "planner" || r.Workspace != WorkspaceRead || len(r.Outcomes) != 2 || len(r.Outputs) != 1 {
		t.Fatalf("unexpected role: %+v", r)
	}
}

func TestParseRoleUnknownField(t *testing.T) {
	_, err := ParseRole([]byte(validRole + "\nwat: true\n"))
	if err == nil {
		t.Fatal("expected unknown-field error")
	}
}

func TestValidateRoleErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"missing id", "prompt: p.md\noutcomes: [a]\nworkspace: read\n", "id is required"},
		{"missing prompt", "id: x\noutcomes: [a]\nworkspace: read\n", "prompt"},
		{"invalid workspace", "id: x\nprompt: p.md\noutcomes: [a]\nworkspace: both\n", "invalid workspace"},
		{"no outcomes", "id: x\nprompt: p.md\nworkspace: read\n", "at least one outcome"},
		{"empty outcome", "id: x\nprompt: p.md\noutcomes: [a, \"\"]\nworkspace: read\n", "outcome must not be empty"},
		{"absolute prompt", "id: x\nprompt: /abs/p.md\noutcomes: [a]\nworkspace: read\n", "must be relative"},
		{"dotdot prompt", "id: x\nprompt: ../p.md\noutcomes: [a]\nworkspace: read\n", "must not contain '..'"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := ParseRole([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			err = r.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}
