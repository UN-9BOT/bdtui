package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// BuildEnvelope renders a deterministic prompt envelope from the fully-resolved
// EnvelopeInput. The output is byte-stable for equal inputs: map keys are
// sorted, no timestamps are embedded, whitespace is normalized. The envelope
// instructs the agent to write a controller-assigned result.json matching the
// role contract and each declared artifact to its assigned path, so the
// machine completion gate is well-defined.
//
// BuildEnvelope rejects if any role-declared output lacks a non-empty
// controller-assigned path, so a missing path cannot disable the mandatory
// artifact invariant downstream.
func BuildEnvelope(in EnvelopeInput) (string, error) {
	if in.Role.ID == "" {
		return "", errors.New("agent: envelope: role id is required")
	}
	if in.RolePrompt == "" {
		return "", errors.New("agent: envelope: role prompt is required")
	}
	if in.OutputPaths.Result == "" {
		return "", errors.New("agent: envelope: result output path is required")
	}
	if len(in.Contract.AllowedOutcomes) == 0 {
		return "", errors.New("agent: envelope: at least one allowed outcome is required")
	}
	if strings.TrimSpace(in.Contract.Schema) == "" {
		return "", errors.New("agent: envelope: result schema is required")
	}

	missing := missingArtifactPaths(in.Contract.DeclaredOutputs, in.OutputPaths)
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", fmt.Errorf("agent: envelope: declared outputs without controller-assigned path: %s", strings.Join(missing, ", "))
	}

	var b strings.Builder

	b.WriteString("# Role\n")
	fmt.Fprintf(&b, "id: %s\n", in.Role.ID)
	if d := strings.TrimSpace(in.Role.Description); d != "" {
		fmt.Fprintf(&b, "description: %s\n", d)
	}
	fmt.Fprintf(&b, "workspace: %s\n", in.Role.Workspace)
	b.WriteString("allowed_outcomes:\n")
	for _, o := range in.Contract.AllowedOutcomes {
		fmt.Fprintf(&b, "  - %s\n", o)
	}
	b.WriteString("declared_outputs:\n")
	outputNames := append([]string(nil), in.Contract.DeclaredOutputs...)
	sort.Strings(outputNames)
	for _, o := range outputNames {
		fmt.Fprintf(&b, "  - %s: %s\n", o, in.OutputPaths.Artifacts[o])
	}
	b.WriteString("prompt: |\n")
	for _, ln := range strings.Split(strings.TrimRight(in.RolePrompt, "\n"), "\n") {
		fmt.Fprintf(&b, "  %s\n", ln)
	}

	b.WriteString("\n# Task\n")
	fmt.Fprintf(&b, "id: %s\n", in.Task.ID)
	fmt.Fprintf(&b, "title: %s\n", in.Task.Title)
	b.WriteString("description: |\n")
	for _, ln := range strings.Split(strings.TrimRight(in.Task.Description, "\n"), "\n") {
		fmt.Fprintf(&b, "  %s\n", ln)
	}

	b.WriteString("\n# Project Instructions\n")
	if len(in.Instructions) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, ins := range in.Instructions {
			if strings.TrimSpace(ins.Name) == "" {
				return "", fmt.Errorf("agent: envelope: instruction name is required")
			}
			fmt.Fprintf(&b, "## %s\n", ins.Name)
			for _, ln := range strings.Split(strings.TrimRight(ins.Content, "\n"), "\n") {
				fmt.Fprintf(&b, "  %s\n", ln)
			}
		}
	}

	b.WriteString("\n# Inputs\n")
	if len(in.Inputs) == 0 {
		b.WriteString("(none)\n")
	} else {
		keys := make([]string, 0, len(in.Inputs))
		for k := range in.Inputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v, err := encodeInputValue(in.Inputs[k])
			if err != nil {
				return "", fmt.Errorf("agent: envelope: input %q: %w", k, err)
			}
			fmt.Fprintf(&b, "- %s: %s\n", k, v)
		}
	}

	b.WriteString("\n# Output Contract\n")
	fmt.Fprintf(&b, "Write your structured result to: %s\n", in.OutputPaths.Result)
	b.WriteString("The result.json MUST be a JSON object of this shape:\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"outcome\": \"<one of allowed_outcomes>\",\n")
	b.WriteString("  \"data\": { ... }\n")
	b.WriteString("}\n")
	b.WriteString("```\n")
	b.WriteString("The `data` object MUST satisfy this JSON Schema:\n")
	b.WriteString("```json\n")
	b.WriteString(strings.TrimRight(in.Contract.Schema, "\n"))
	b.WriteString("\n```\n")
	b.WriteString("Write declared_outputs to their assigned paths.\n")

	b.WriteString("\n# Completion\n")
	b.WriteString("Machine completion is gated on:\n")
	b.WriteString("- the agent process exits cleanly,\n")
	b.WriteString("- result.json exists at the assigned path, parses as JSON, and satisfies the contract above,\n")
	b.WriteString("- every declared_output is written to its assigned path.\n")
	b.WriteString("An empty worktree or an empty git diff does NOT waive these requirements.\n")

	return b.String(), nil
}

// missingArtifactPaths returns the declared output names whose
// controller-assigned path is empty or missing.
func missingArtifactPaths(declared []string, paths OutputPaths) []string {
	var missing []string
	for _, name := range declared {
		if strings.TrimSpace(paths.Artifacts[name]) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

// encodeInputValue renders a declared input value as a stable, single-line
// string. Strings render unquoted when they are simple; everything else
// renders as compact JSON.
func encodeInputValue(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "null", nil
	case string:
		if isSimpleScalar(x) {
			return x, nil
		}
		b, err := json.Marshal(x)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case bool, int, int64, float64:
		return fmt.Sprintf("%v", x), nil
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

func isSimpleScalar(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.' || r == ':' || r == '/':
		default:
			return false
		}
	}
	return true
}