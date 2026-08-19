package agent

import (
	"encoding/json"
	"testing"
)

const dataSchema = `{
  "type": "object",
  "required": ["plan"],
  "properties": {"plan": {"type": "string"}},
  "additionalProperties": true
}`

func validResultContract() ResultContract {
	return ResultContract{
		Schema:          dataSchema,
		AllowedOutcomes: []string{"planned", "needs_clarification"},
		DeclaredOutputs: []string{"plan"},
	}
}

// buildResultJSON returns a valid {"outcome", "data"} envelope.
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
	body := buildResultJSON(t, "planned", map[string]any{"plan": "step 1"})
	res := Result{ResultJSON: body, Artifacts: map[string][]byte{"plan": []byte("step 1")}}
	got, err := CheckCompletion(res, validResultContract())
	if err != nil {
		t.Fatalf("CheckCompletion: %v", err)
	}
	if got.Outcome != "planned" {
		t.Fatalf("Outcome=%q", got.Outcome)
	}
}

func TestCheckCompletionRejectsError(t *testing.T) {
	body := buildResultJSON(t, "planned", map[string]any{"plan": "x"})
	if _, err := CheckCompletion(Result{IsError: true, ResultJSON: body}, validResultContract()); err == nil {
		t.Fatal("expected IsError rejection")
	}
}

func TestCheckCompletionRejectsMissingResult(t *testing.T) {
	if _, err := CheckCompletion(Result{}, validResultContract()); err == nil {
		t.Fatal("expected missing-result rejection")
	}
}

func TestCheckCompletionRejectsBadJSON(t *testing.T) {
	res := Result{ResultJSON: []byte(`{not json`)}
	if _, err := CheckCompletion(res, validResultContract()); err == nil {
		t.Fatal("expected bad-JSON rejection")
	}
}

func TestCheckCompletionRejectsMissingOutcome(t *testing.T) {
	res := Result{ResultJSON: []byte(`{"data":{"plan":"x"}}`)}
	if _, err := CheckCompletion(res, validResultContract()); err == nil {
		t.Fatal("expected missing-outcome rejection")
	}
}

func TestCheckCompletionRejectsDisallowedOutcome(t *testing.T) {
	body := buildResultJSON(t, "rogue", map[string]any{"plan": "x"})
	res := Result{ResultJSON: body}
	if _, err := CheckCompletion(res, validResultContract()); err == nil {
		t.Fatal("expected disallowed-outcome rejection")
	}
}

func TestCheckCompletionRejectsNonStringOutcome(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"outcome": 42, "data": map[string]any{"plan": "x"}})
	res := Result{ResultJSON: body}
	if _, err := CheckCompletion(res, validResultContract()); err == nil {
		t.Fatal("expected non-string-outcome rejection")
	}
}

func TestCheckCompletionRejectsMissingData(t *testing.T) {
	res := Result{ResultJSON: []byte(`{"outcome":"planned"}`)}
	if _, err := CheckCompletion(res, validResultContract()); err == nil {
		t.Fatal("expected missing-data rejection")
	}
}

func TestCheckCompletionRejectsNullData(t *testing.T) {
	res := Result{ResultJSON: []byte(`{"outcome":"planned","data":null}`)}
	if _, err := CheckCompletion(res, validResultContract()); err == nil {
		t.Fatal("expected null-data rejection")
	}
}

func TestCheckCompletionRejectsDataSchemaMismatch(t *testing.T) {
	body := buildResultJSON(t, "planned", map[string]any{"plan": 42}) // plan must be string
	res := Result{ResultJSON: body}
	if _, err := CheckCompletion(res, validResultContract()); err == nil {
		t.Fatal("expected data-schema rejection")
	}
}

func TestCheckCompletionRejectsMissingArtifact(t *testing.T) {
	body := buildResultJSON(t, "planned", map[string]any{"plan": "x"})
	res := Result{ResultJSON: body} // no artifact
	if _, err := CheckCompletion(res, validResultContract()); err == nil {
		t.Fatal("expected missing-artifact rejection")
	}
}

func TestCheckCompletionRejectsBadSchema(t *testing.T) {
	c := validResultContract()
	c.Schema = "{not json"
	body := buildResultJSON(t, "planned", map[string]any{"plan": "x"})
	if _, err := CheckCompletion(Result{ResultJSON: body}, c); err == nil {
		t.Fatal("expected bad-schema rejection")
	}
}