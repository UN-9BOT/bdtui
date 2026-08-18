package agent

import (
	"encoding/json"
	"testing"
)

const completionSchema = `{
  "type": "object",
  "required": ["outcome"],
  "properties": {
    "outcome": {"type": "string"},
    "plan":    {"type": "string"}
  },
  "additionalProperties": true
}`

func validContract() ResultContract {
	return ResultContract{
		Schema:          completionSchema,
		AllowedOutcomes: []string{"planned", "needs_clarification"},
	}
}

func TestCheckCompletionValid(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"outcome": "planned", "plan": "step 1"})
	res := Result{
		ResultJSON: body,
		Artifacts:  map[string][]byte{"plan": []byte("step 1")},
	}
	got, err := CheckCompletion(res, validContract(), []string{"plan"})
	if err != nil {
		t.Fatalf("CheckCompletion: %v", err)
	}
	if got.Outcome != "planned" {
		t.Fatalf("Outcome=%q want planned", got.Outcome)
	}
}

func TestCheckCompletionRejectsError(t *testing.T) {
	res := Result{IsError: true, ResultJSON: []byte(`{"outcome":"planned"}`)}
	if _, err := CheckCompletion(res, validContract(), nil); err == nil {
		t.Fatal("expected IsError rejection")
	}
}

func TestCheckCompletionRejectsMissingResult(t *testing.T) {
	res := Result{}
	if _, err := CheckCompletion(res, validContract(), nil); err == nil {
		t.Fatal("expected missing-result rejection")
	}
}

func TestCheckCompletionRejectsBadSchema(t *testing.T) {
	res := Result{ResultJSON: []byte(`{"plan":"x"}`)} // missing outcome
	if _, err := CheckCompletion(res, validContract(), nil); err == nil {
		t.Fatal("expected schema rejection")
	}
}

func TestCheckCompletionRejectsMalformedJSON(t *testing.T) {
	res := Result{ResultJSON: []byte(`{not json`)}
	if _, err := CheckCompletion(res, validContract(), nil); err == nil {
		t.Fatal("expected malformed-JSON rejection")
	}
}

func TestCheckCompletionRejectsDisallowedOutcome(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"outcome": "rogue"})
	res := Result{ResultJSON: body}
	if _, err := CheckCompletion(res, validContract(), nil); err == nil {
		t.Fatal("expected disallowed-outcome rejection")
	}
}

func TestCheckCompletionRejectsNonStringOutcome(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"outcome": 42})
	res := Result{ResultJSON: body}
	if _, err := CheckCompletion(res, validContract(), nil); err == nil {
		t.Fatal("expected non-string-outcome rejection")
	}
}

func TestCheckCompletionRejectsMissingArtifact(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"outcome": "planned"})
	res := Result{ResultJSON: body}
	if _, err := CheckCompletion(res, validContract(), []string{"plan"}); err == nil {
		t.Fatal("expected missing-artifact rejection")
	}
}

func TestCheckCompletionRejectsEmptyArtifact(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"outcome": "planned"})
	res := Result{
		ResultJSON: body,
		Artifacts:  map[string][]byte{"plan": nil},
	}
	if _, err := CheckCompletion(res, validContract(), []string{"plan"}); err == nil {
		t.Fatal("expected empty-artifact rejection")
	}
}

func TestCheckCompletionRejectsBadSchemaText(t *testing.T) {
	bad := validContract()
	bad.Schema = "{not json"
	res := Result{ResultJSON: []byte(`{"outcome":"planned"}`)}
	if _, err := CheckCompletion(res, bad, nil); err == nil {
		t.Fatal("expected bad-schema rejection")
	}
}
