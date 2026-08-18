package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// SessionStore persists the mapping from a controller-side session key
// (typically "<run-id>/<role-id>") to the underlying agent session id. The
// controller owns persistence so a restart can re-attach to the same agent
// session; the adapter only reads/writes the mapping.
type SessionStore interface {
	// Get returns the agent session id stored for key, and whether it exists.
	Get(key string) (sessionID string, ok bool)
	// Put stores agent session id for key.
	Put(key string, sessionID string)
}

// MakiAdapter implements Adapter on top of the Maki CLI. It hides the CLI and
// the JSON wire format from the controller and translates a controller-built
// Request into a single `maki --print --output-format json` invocation.
//
// MVP session model: the adapter maintains one Maki session per SessionKey
// (typically a (run, role) pair) via SessionStore, so revise/review loops for
// the same role may reuse context. Maki's `--print` mode does not currently
// honor `-s/--session` (it is parsed but ignored; resume is supported only
// in the TUI and SDK modes), so this MVP captures the session_id from each
// `maki --print` result and exposes it via Result.SessionID; true in-process
// session reuse would require switching the adapter to Maki's SDK/ACP mode,
// which is an extension point left to a future task.
type MakiAdapter struct {
	runner CommandRunner
	bin    string
	sess   SessionStore
}

// NewMakiAdapter builds a MakiAdapter. bin is the path to the `maki` binary
// (use "maki" to resolve via PATH). sessions is the session store; pass an
// in-memory or persistent implementation. If sessions is nil, a process-local
// in-memory store is used.
func NewMakiAdapter(runner CommandRunner, bin string, sessions SessionStore) *MakiAdapter {
	if runner == nil {
		panic("agent: MakiAdapter: runner is nil")
	}
	if bin == "" {
		bin = "maki"
	}
	if sessions == nil {
		sessions = newMemorySessionStore()
	}
	return &MakiAdapter{runner: runner, bin: bin, sess: sessions}
}

// makiPrintResult mirrors the wire format documented in `maki --print
// --output-format json`. Wire format intentionally matches Claude Code; we
// keep only the fields the controller actually needs.
type makiPrintResult struct {
	Type       string `json:"type"`
	Subtype    string `json:"subtype"`
	IsError    bool   `json:"is_error"`
	StopReason string `json:"stop_reason"`
	Result     string `json:"result"`
	SessionID  string `json:"session_id"`
}

// Run executes the Maki invocation described by req. The prompt is passed
// positionally so the process sees it on argv (deterministic, no stdin
// ambiguity). After the process completes the adapter reads the
// controller-assigned result.json and declared artifacts from disk and
// returns them in Result. Process-level errors (non-zero exit, malformed
// JSON) are surfaced via Result.IsError and the returned error.
func (a *MakiAdapter) Run(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return Result{IsError: true}, errors.New("agent: MakiAdapter: prompt is required")
	}
	if strings.TrimSpace(req.WorkingDir) == "" {
		return Result{IsError: true}, errors.New("agent: MakiAdapter: working dir is required")
	}
	if strings.TrimSpace(req.OutputPaths.Result) == "" {
		return Result{IsError: true}, errors.New("agent: MakiAdapter: result output path is required")
	}

	args := []string{"--print", "--output-format", "json", "--exit-on-done", req.Prompt}

	stdout, stderr, runErr := a.runner.Run(ctx, a.bin, args, RunOpts{Dir: req.WorkingDir})

	// Build the process-level Result first; populate result.json and artifacts
	// from disk regardless of process outcome so a structured failure still
	// carries whatever the agent did manage to write.
	res := Result{}

	if len(stdout) == 0 {
		res.IsError = true
		if runErr != nil {
			return res, fmt.Errorf("agent: MakiAdapter: maki process failed: %w (stderr=%s)", runErr, truncate(stderr))
		}
		return res, errors.New("agent: MakiAdapter: maki produced no output")
	}

	var pr makiPrintResult
	if err := json.Unmarshal(stdout, &pr); err != nil {
		res.IsError = true
		res.Raw = string(stdout)
		return res, fmt.Errorf("agent: MakiAdapter: parse maki output: %w (stderr=%s)", err, truncate(stderr))
	}

	res.SessionID = pr.SessionID
	res.StopReason = pr.StopReason
	res.IsError = pr.IsError
	res.Raw = pr.Result

	if req.SessionKey != "" && res.SessionID != "" {
		a.sess.Put(req.SessionKey, res.SessionID)
	}

	if !res.IsError {
		res.ResultJSON = readFile(req.OutputPaths.Result)
		res.Artifacts = make(map[string][]byte, len(req.OutputPaths.Artifacts))
		for name, path := range req.OutputPaths.Artifacts {
			if path == "" {
				continue
			}
			if body := readFile(path); len(body) > 0 {
				res.Artifacts[name] = body
			}
		}
	}

	if runErr != nil && !res.IsError {
		// The process reported success but exited non-zero (e.g. signal). Flag
		// it and surface the underlying error.
		res.IsError = true
		return res, fmt.Errorf("agent: MakiAdapter: maki exited non-zero: %w (stderr=%s)", runErr, truncate(stderr))
	}

	return res, nil
}

func readFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

func truncate(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

// memorySessionStore is a goroutine-safe in-process SessionStore.
type memorySessionStore struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemorySessionStore() *memorySessionStore { return &memorySessionStore{m: map[string]string{}} }

func (s *memorySessionStore) Get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	return v, ok
}

func (s *memorySessionStore) Put(key, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = sessionID
}
