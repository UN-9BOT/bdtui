// Package agent defines the generic, provider-agnostic boundary between the
// orchestrator controller and a concrete agent runner (the MVP being Maki),
// plus the deterministic prompt envelope and the completion checks that gate a
// step attempt on machine-verifiable evidence.
//
// The boundary has three layers so a future runtime (HerdrRuntime) can own the
// process lifecycle without changing the controller or the adapter:
//
//	Controller -> RunAgent(adapter, runtime, req)
//	             |                       |
//	          MakiAdapter             Runtime (HerdrRuntime / ExecRuntime)
//	             |                       |
//	      build invocation        spawn / wait / stop / inspect / reattach
//	      parse wire format       (controller-allocated ExecutionID)
//
// Workflow authoring is human-driven: this package only reads and validates
// definitions; it never creates or edits them. Agents do not create or edit
// workflows either; they execute a fully rendered prompt and produce a
// controller-assigned result.json plus any declared artifacts.
package agent

import (
	"errors"
	"fmt"
	"strings"

	"bdtui/internal/workflow"
)

// ErrLostExecution is returned by RunAgent when a reattach finds no live
// process for the given ExecutionID. This is the crash-recovery signal: the
// controller persists the execution_id before spawn, so a NotFound on inspect
// after a daemon restart means the live process is gone. The controller
// resolves this into needs_attention (writer) or technical retry (reader)
// per the runtime/recovery contract.
var ErrLostExecution = errors.New("agent: execution lost (no live runtime record)")

// Request is a provider-agnostic request to run a single agent step. The
// controller builds the deterministic prompt envelope and assigns the output
// paths; the runtime owns the process lifecycle; the adapter only translates
// the request into the underlying CLI/protocol.
type Request struct {
	// ExecutionID is the durable runtime-side identity for this attempt. The
	// controller MUST allocate it (UUID), persist it in the orch store
	// BEFORE invoking RunAgent, and pass it here on every call (fresh and
	// reattach). The runtime uses it for spawn and reattach.
	ExecutionID string

	// Reattach signals that ExecutionID refers to an already-spawned
	// execution that the controller wants to re-attach to (after a daemon
	// restart or async dispatch). RunAgent will Inspect+Wait instead of
	// Spawn when true. False (the default) is a fresh spawn.
	Reattach bool

	// SessionKey identifies a reusable agent session. MVP keeps one session
	// per (run, role) so revise/review loops for the same role may reuse the
	// agent's conversation context. The adapter maps this to the underlying
	// agent protocol's resume primitive.
	SessionKey string

	// Prompt is the deterministic, fully-rendered prompt envelope.
	Prompt string

	// WorkingDir is the Git worktree the agent operates in.
	WorkingDir string

	// OutputPaths are controller-assigned absolute paths inside Run storage
	// where the agent must write its structured result and declared artifacts.
	OutputPaths OutputPaths

	// Contract is the resolved, immutable completion contract for this
	// attempt. It is constructed only via ResolveContract; the controller
	// cannot assemble the parts independently and therefore cannot bypass
	// the mandatory-artifact invariant by omitting DeclaredOutputs.
	Contract ResultContract
}

// OutputPaths are the controller-assigned absolute paths inside Run storage
// where the agent must write its structured result and declared artifacts.
type OutputPaths struct {
	Result    string
	Artifacts map[string]string
}

// ResultContract is the single, resolved contract for a step attempt. The
// fields are unexported so the contract can only be constructed via
// ResolveContract (which derives Outcomes and Outputs from the
// authoritative RoleContract). This makes the mandatory-artifact invariant
// structurally impossible to bypass from outside the package.
//
// The JSON Schema validates the result `data` object only; the `outcome`
// field is a control-plane concern validated separately by the controller.
type ResultContract struct {
	roleID          string
	schema          string
	allowedOutcomes []string
	declaredOutputs []string
}

// RoleID returns the role id this contract was resolved for.
func (c ResultContract) RoleID() string { return c.roleID }

// Schema returns the JSON Schema text for the result.json `data` object.
func (c ResultContract) Schema() string { return c.schema }

// AllowedOutcomes returns the set of semantic outcomes the role may report.
func (c ResultContract) AllowedOutcomes() []string {
	return append([]string(nil), c.allowedOutcomes...)
}

// DeclaredOutputs returns the resolved set of role-declared artifact names.
func (c ResultContract) DeclaredOutputs() []string {
	return append([]string(nil), c.declaredOutputs...)
}

// ResolveContract builds the immutable ResultContract for a step attempt
// from the resolved role contract and the result_schema content. Outcomes
// and Outputs are taken from the role; the controller cannot supply its own
// disjoint lists, so the mandatory-artifact invariant cannot be bypassed.
// role.Validate() is enforced first, so a malformed role (e.g. nil
// Outputs) cannot mint a contract without mandatory artifacts.
func ResolveContract(role workflow.RoleContract, schemaContent string) (ResultContract, error) {
	if err := role.Validate(); err != nil {
		return ResultContract{}, fmt.Errorf("agent: ResolveContract: role: %w", err)
	}
	if strings.TrimSpace(schemaContent) == "" {
		return ResultContract{}, errors.New("agent: ResolveContract: schema is required")
	}

	seenO := map[string]bool{}
	outcomes := make([]string, 0, len(role.Outcomes))
	for _, o := range role.Outcomes {
		if strings.TrimSpace(o) == "" || seenO[o] {
			continue
		}
		seenO[o] = true
		outcomes = append(outcomes, o)
	}
	if len(outcomes) == 0 {
		return ResultContract{}, errors.New("agent: ResolveContract: role has no outcomes")
	}

	seenD := map[string]bool{}
	outputs := make([]string, 0, len(role.Outputs))
	for _, o := range role.Outputs {
		if strings.TrimSpace(o) == "" || seenD[o] {
			continue
		}
		seenD[o] = true
		outputs = append(outputs, o)
	}

	return ResultContract{
		roleID:          role.ID,
		schema:          schemaContent,
		allowedOutcomes: outcomes,
		declaredOutputs: outputs,
	}, nil
}

// consistentWith reports whether c was resolved for the exact same role it
// is being paired with. It checks the role id, allowed outcomes, and
// declared outputs. BuildEnvelope and the production reconciler MUST call
// this before using a ResultContract; ResolveContract makes it hold by
// construction for the originating role.
func (c ResultContract) consistentWith(role workflow.RoleContract) error {
	if c.roleID != role.ID {
		return fmt.Errorf("agent: ResultContract: role_id mismatch (contract=%q, role=%q)", c.roleID, role.ID)
	}
	if !sameStringSet(c.allowedOutcomes, role.Outcomes) {
		return errors.New("agent: ResultContract: allowed_outcomes do not match role.outcomes")
	}
	if !sameStringSet(c.declaredOutputs, role.Outputs) {
		return errors.New("agent: ResultContract: declared_outputs do not match role.outputs")
	}
	return nil
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]bool{}
	for _, v := range a {
		m[v] = true
	}
	for _, v := range b {
		if !m[v] {
			return false
		}
	}
	return true
}

// TaskSnapshot is the immutable snapshot of the source Kanban task for a run.
type TaskSnapshot struct {
	ID          string
	Title       string
	Description string
}

// ProjectInstruction is a snapshotted project instruction file.
type ProjectInstruction struct {
	Name    string
	Content string
}

// EnvelopeInput is the fully-resolved input the controller passes to
// BuildEnvelope.
type EnvelopeInput struct {
	Role         workflow.RoleContract
	RolePrompt   string
	Task         TaskSnapshot
	Instructions []ProjectInstruction
	Inputs       map[string]any
	OutputPaths  OutputPaths
	Contract     ResultContract
}

// Result is the raw outcome of a single agent invocation.
type Result struct {
	ExecutionID string
	SessionID   string
	StopReason  string
	IsError     bool
	ResultJSON  []byte
	Artifacts   map[string][]byte
	Raw         string
}

// Completion is the validated completion of a step attempt.
type Completion struct {
	Outcome string
}