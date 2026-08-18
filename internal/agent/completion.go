package agent

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// CheckCompletion validates that the adapter-reported Result satisfies the
// machine-completion contract: result.json exists, parses, satisfies the
// contract schema, declares an outcome that is in Contract.AllowedOutcomes,
// and every role-declared output has a non-empty artifact produced by the
// adapter. Empty repository content or an empty Git diff never waive these
// requirements; this gate is intentionally independent of worktree state.
//
// On success it returns the validated Completion. On failure it returns an
// error that names the first violation found.
func CheckCompletion(r Result, contract ResultContract, declaredOutputs []string) (Completion, error) {
	if r.IsError {
		return Completion{}, errors.New("agent: completion: agent process reported error")
	}
	if len(r.ResultJSON) == 0 {
		return Completion{}, errors.New("agent: completion: result.json is missing or empty")
	}

	rs, err := resolveSchema(contract.Schema)
	if err != nil {
		return Completion{}, fmt.Errorf("agent: completion: resolve schema: %w", err)
	}

	var raw any
	if err := json.Unmarshal(r.ResultJSON, &raw); err != nil {
		return Completion{}, fmt.Errorf("agent: completion: result.json is not valid JSON: %w", err)
	}
	if err := rs.Validate(raw); err != nil {
		return Completion{}, fmt.Errorf("agent: completion: result.json does not satisfy schema: %w", err)
	}

	outcome, err := extractOutcome(raw)
	if err != nil {
		return Completion{}, fmt.Errorf("agent: completion: %w", err)
	}
	if !contains(contract.AllowedOutcomes, outcome) {
		return Completion{}, fmt.Errorf("agent: completion: outcome %q is not in allowed_outcomes", outcome)
	}

	for _, name := range declaredOutputs {
		body, ok := r.Artifacts[name]
		if !ok || len(body) == 0 {
			return Completion{}, fmt.Errorf("agent: completion: declared output %q is missing", name)
		}
	}

	return Completion{Outcome: outcome}, nil
}

// resolveSchema compiles the contract schema text and resolves internal
// references so it can validate an instance.
func resolveSchema(schemaText string) (*jsonschema.Resolved, error) {
	var s jsonschema.Schema
	if err := json.Unmarshal([]byte(schemaText), &s); err != nil {
		return nil, err
	}
	return s.Resolve(nil)
}

// extractOutcome pulls the `outcome` field out of a parsed result.json. The
// field is required to be a non-empty string; the JSON Schema check above
// enforces the broader shape, this is a precise extraction with a precise
// error for the common case where the agent forgot the field.
func extractOutcome(raw any) (string, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return "", errors.New("result.json must be a JSON object")
	}
	v, present := obj["outcome"]
	if !present {
		return "", errors.New("result.json is missing the `outcome` field")
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", errors.New("result.json `outcome` field must be a non-empty string")
	}
	return s, nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
