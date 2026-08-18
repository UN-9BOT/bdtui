package orch

// RunStatus is the MVP lifecycle of a Run.
type RunStatus string

const (
	RunQueued         RunStatus = "queued"
	RunRunning        RunStatus = "running"
	RunWaitingHuman   RunStatus = "waiting_human"
	RunNeedsAttention RunStatus = "needs_attention"
	RunCompleted      RunStatus = "completed"
	RunFailed         RunStatus = "failed"
	RunCancelled      RunStatus = "cancelled"
)

// Valid reports whether s is one of the defined RunStatus values.
func (s RunStatus) Valid() bool {
	switch s {
	case RunQueued, RunRunning, RunWaitingHuman, RunNeedsAttention, RunCompleted, RunFailed, RunCancelled:
		return true
	default:
		return false
	}
}

// Terminal reports whether a Run in this status will not advance further.
func (s RunStatus) Terminal() bool {
	return s == RunCompleted || s == RunFailed || s == RunCancelled
}

// StepAttemptStatus is the lifecycle of a single step attempt within a Run.
type StepAttemptStatus string

const (
	StepQueued         StepAttemptStatus = "queued"
	StepRunning        StepAttemptStatus = "running"
	StepWaitingHuman   StepAttemptStatus = "waiting_human"
	StepNeedsAttention StepAttemptStatus = "needs_attention"
	StepCompleted      StepAttemptStatus = "completed"
	StepFailed         StepAttemptStatus = "failed"
	StepCancelled      StepAttemptStatus = "cancelled"
)

func (s StepAttemptStatus) Valid() bool {
	switch s {
	case StepQueued, StepRunning, StepWaitingHuman, StepNeedsAttention, StepCompleted, StepFailed, StepCancelled:
		return true
	default:
		return false
	}
}

func (s StepAttemptStatus) Terminal() bool {
	return s == StepCompleted || s == StepFailed || s == StepCancelled
}

// ExecutionStatus is the lifecycle of a concrete process/agent execution.
type ExecutionStatus string

const (
	ExecQueued         ExecutionStatus = "queued"
	ExecRunning        ExecutionStatus = "running"
	ExecWaitingHuman   ExecutionStatus = "waiting_human"
	ExecNeedsAttention ExecutionStatus = "needs_attention"
	ExecCompleted      ExecutionStatus = "completed"
	ExecFailed         ExecutionStatus = "failed"
	ExecCancelled      ExecutionStatus = "cancelled"
)

func (s ExecutionStatus) Valid() bool {
	switch s {
	case ExecQueued, ExecRunning, ExecWaitingHuman, ExecNeedsAttention, ExecCompleted, ExecFailed, ExecCancelled:
		return true
	default:
		return false
	}
}

func (s ExecutionStatus) Terminal() bool {
	return s == ExecCompleted || s == ExecFailed || s == ExecCancelled
}

// ExecutionKind discriminates what a single Execution drives.
type ExecutionKind string

const (
	KindAgent ExecutionKind = "agent"
	KindHuman ExecutionKind = "human"
	KindEnd   ExecutionKind = "end"
)

func (k ExecutionKind) Valid() bool {
	switch k {
	case KindAgent, KindHuman, KindEnd:
		return true
	default:
		return false
	}
}

// HumanInputStatus is the lifecycle of a prompt awaiting human action.
type HumanInputStatus string

const (
	HumanPending   HumanInputStatus = "pending"
	HumanAnswered  HumanInputStatus = "answered"
	HumanCancelled HumanInputStatus = "cancelled"
	HumanTimedOut  HumanInputStatus = "timed_out"
)

func (s HumanInputStatus) Valid() bool {
	switch s {
	case HumanPending, HumanAnswered, HumanCancelled, HumanTimedOut:
		return true
	default:
		return false
	}
}

// LaunchIntentStatus is the lifecycle of a pending request to launch a Run.
type LaunchIntentStatus string

const (
	LaunchPending   LaunchIntentStatus = "pending"
	LaunchAccepted  LaunchIntentStatus = "accepted"
	LaunchRejected  LaunchIntentStatus = "rejected"
	LaunchCancelled LaunchIntentStatus = "cancelled"
)

func (s LaunchIntentStatus) Valid() bool {
	switch s {
	case LaunchPending, LaunchAccepted, LaunchRejected, LaunchCancelled:
		return true
	default:
		return false
	}
}

// Event types are open-ended; these constants cover the MVP transitions.
const (
	EventProjectUpserted = "project.upserted"
	EventRunCreated      = "run.created"
	EventRunTransition   = "run.transition"
	EventStepCreated     = "step.created"
	EventStepTransition  = "step.transition"
	EventExecCreated     = "execution.created"
	EventExecTransition  = "execution.transition"
	EventHumanRequested  = "human.input_requested"
	EventHumanAnswered   = "human.input_answered"
	EventIntentCreated   = "launch_intent.created"
	EventIntentResolved  = "launch_intent.resolved"
)
