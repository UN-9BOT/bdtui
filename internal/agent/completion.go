package agent

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// CheckCompletion validates that the adapter-reported Result satisfies the
// machine-completion contract: result.json exists, parses, has its `outcome`
// in Contract.AllowedOutcomes, has a `data` object that satisfies the role
// contract schema, and every Contract.DeclaredOutput has a non-empty
// artifact produced by the adapter.
//
// The contract is a single value, not a list of independent sources, so the
// mandatory-artifact invariant cannot be disabled by the caller omitting an
// argument. Empty repository content or an empty Git diff never waive these
// requirements; this gate is intentionally independent of worktree state.
func CheckCompletion(r Result, contract ResultContract) (Completion, error) {
	if r.IsError {
		return Completion{}, errors.New("agent: completion: agent process reported error")
	}
	if len(r.ResultJSON) == 0 {
		return Completion{}, errors.New("agent: completion: result.json is missing or empty")
	}

	rs, err := resolveSchema(contract.schema)
	if err != nil {
		return Completion{}, fmt.Errorf("agent: completion: resolve schema: %w", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(r.ResultJSON, &raw); err != nil {
		return Completion{}, fmt.Errorf("agent: completion: result.json is not valid JSON: %w", err)
	}

	outcome, err := extractOutcome(raw)
	if err != nil {
		return Completion{}, fmt.Errorf("agent: completion: %w", err)
	}
	if !contains(contract.allowedOutcomes, outcome) {
		return Completion{}, fmt.Errorf("agent: completion: outcome %q is not in allowed_outcomes", outcome)
	}

	data, hasData := raw["data"]
	if !hasData {
		return Completion{}, errors.New("agent: completion: result.json is missing the `data` field")
	}
	if data == nil {
		return Completion{}, errors.New("agent: completion: result.json `data` field must not be null")
	}
	if err := rs.Validate(data); err != nil {
		return Completion{}, fmt.Errorf("agent: completion: result.json `data` does not satisfy schema: %w", err)
	}

	for _, name := range contract.declaredOutputs {
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

// extractOutcome pulls the `outcome` field out of the parsed result.json
// envelope. The field is required to be a non-empty string; the JSON Schema
// check covers the `data` shape, this covers the control-plane field.
func extractOutcome(raw map[string]any) (string, error) {
	v, present := raw["outcome"]
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