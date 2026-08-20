package agent

import (
	"encoding/json"
	"testing"

	"bdtui/internal/workflow"
)

const dataSchemaText = `{"type":"object","required":["plan"],"properties":{"plan":{"type":"string"}}}`

func validCompletionRole() workflow.RoleContract {
	return workflow.RoleContract{
		ID:           "impl",
		Prompt:       "prompts/impl.md",
		Workspace:    workflow.WorkspaceWrite,
		Outcomes:     []string{"planned", "needs_clarification"},
		Outputs:      []string{"plan"},
		ResultSchema: "schemas/result.json",
	}
}

func validCompletionContract(t *testing.T) ResultContract {
	t.Helper()
	return resolvedContract(t, validCompletionRole())
}

func buildResultJSON(t *testing.T, outcome string, data map[string]any) []byte {
	t.Helper()
	body := map[string]any{"outcome": outcome, "data": data}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCheckCompletionValid(t *testing.T) {
	c := validCompletionContract(t)
	body := buildResultJSON(t, "planned", map[string]any{"plan": "step 1"})
	res := Result{ResultJSON: body, Artifacts: map[string][]byte{"plan": []byte("step 1")}}
	got, err := CheckCompletion(res, c)
	if err != nil {
		t.Fatalf("CheckCompletion: %v", err)
	}
	if got.Outcome != "planned" {
		t.Fatalf("Outcome=%q", got.Outcome)
	}
}

func TestCheckCompletionRejectsError(t *testing.T) {
	body := buildResultJSON(t, "planned", map[string]any{"plan": "x"})
	if _, err := CheckCompletion(Result{IsError: true, ResultJSON: body}, validCompletionContract(t)); err == nil {
		t.Fatal("expected IsError rejection")
	}
}

func TestCheckCompletionRejectsMissingResult(t *testing.T) {
	if _, err := CheckCompletion(Result{}, validCompletionContract(t)); err == nil {
		t.Fatal("expected missing-result rejection")
	}
}

func TestCheckCompletionRejectsBadJSON(t *testing.T) {
	if _, err := CheckCompletion(Result{ResultJSON: []byte(`{not json`)}, validCompletionContract(t)); err == nil {
		t.Fatal("expected bad-JSON rejection")
	}
}

func TestCheckCompletionRejectsMissingOutcome(t *testing.T) {
	if _, err := CheckCompletion(Result{ResultJSON: []byte(`{"data":{"plan":"x"}}`)}, validCompletionContract(t)); err == nil {
		t.Fatal("expected missing-outcome rejection")
	}
}

func TestCheckCompletionRejectsDisallowedOutcome(t *testing.T) {
	body := buildResultJSON(t, "rogue", map[string]any{"plan": "x"})
	if _, err := CheckCompletion(Result{ResultJSON: body}, validCompletionContract(t)); err == nil {
		t.Fatal("expected disallowed-outcome rejection")
	}
}

func TestCheckCompletionRejectsNonStringOutcome(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"outcome": 42, "data": map[string]any{"plan": "x"}})
	if _, err := CheckCompletion(Result{ResultJSON: body}, validCompletionContract(t)); err == nil {
		t.Fatal("expected non-string-outcome rejection")
	}
}

func TestCheckCompletionRejectsMissingData(t *testing.T) {
	if _, err := CheckCompletion(Result{ResultJSON: []byte(`{"outcome":"planned"}`)}, validCompletionContract(t)); err == nil {
		t.Fatal("expected missing-data rejection")
	}
}

func TestCheckCompletionRejectsNullData(t *testing.T) {
	if _, err := CheckCompletion(Result{ResultJSON: []byte(`{"outcome":"planned","data":null}`)}, validCompletionContract(t)); err == nil {
		t.Fatal("expected null-data rejection")
	}
}

func TestCheckCompletionRejectsDataSchemaMismatch(t *testing.T) {
	body := buildResultJSON(t, "planned", map[string]any{"plan": 42})
	if _, err := CheckCompletion(Result{ResultJSON: body}, validCompletionContract(t)); err == nil {
		t.Fatal("expected data-schema rejection")
	}
}

func TestCheckCompletionRejectsMissingArtifact(t *testing.T) {
	body := buildResultJSON(t, "planned", map[string]any{"plan": "x"})
	if _, err := CheckCompletion(Result{ResultJSON: body}, validCompletionContract(t)); err == nil {
		t.Fatal("expected missing-artifact rejection")
	}
}

func TestCheckCompletionRejectsBadSchema(t *testing.T) {
	c := validCompletionContract(t)
	c.schema = "{not json"
	body := buildResultJSON(t, "planned", map[string]any{"plan": "x"})
	if _, err := CheckCompletion(Result{ResultJSON: body}, c); err == nil {
		t.Fatal("expected bad-schema rejection")
	}
}