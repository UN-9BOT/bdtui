package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Closure is the full dependency set of a workflow at Run start: the workflow
// itself plus the content of every referenced role prompt, prompt, JSON
// Schema, and controller-discovered project instruction file. Keys are
// relative paths; values are raw file contents.
type Closure struct {
	Spec  WorkflowSpec
	Files map[string]string
}

// Snapshot is an immutable, content-addressed workflow dependency closure.
type Snapshot struct {
	// Ref is the hex SHA-256 of JSON.
	Ref string
	// JSON is the canonical snapshot document.
	JSON string
}

// BuildSnapshot validates the workflow and compiles a canonical, deterministic
// snapshot of the workflow plus its dependency files. The ref is the hex
// SHA-256 of the canonical JSON, matching Run.WorkflowSnapshotRef.
func BuildSnapshot(c Closure) (Snapshot, error) {
	if err := c.Spec.Validate(); err != nil {
		return Snapshot{}, err
	}
	if c.Files == nil {
		c.Files = map[string]string{}
	}

	payload := struct {
		Workflow WorkflowSpec      `json:"workflow"`
		Files    map[string]string `json:"files"`
	}{
		Workflow: c.Spec.forJSON(),
		Files:    c.Files,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return Snapshot{}, err
	}
	jsonStr := string(b)
	sum := sha256.Sum256([]byte(jsonStr))
	return Snapshot{Ref: hex.EncodeToString(sum[:]), JSON: jsonStr}, nil
}
