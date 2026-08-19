package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// SessionStore persists the mapping from a controller-side session key
// (typically "<run-id>/<role-id>") to the underlying agent session id. The
// adapter uses it to resume sessions across calls.
type SessionStore interface {
	Get(key string) (sessionID string, ok bool)
	Put(key string, sessionID string)
}

// MakiAdapter implements Adapter on top of Maki's SDK streaming mode. It
// hides the CLI flags, the stream-json wire format, and the session resume
// primitive from the controller.
//
// Wire protocol: each Maki stdout line is a JSON object with a `type` field
// and a top-level `session_id`. We send a single `{"type":"user","message":
// {"content":"<prompt>"}}` message on stdin and read the JSONL stream until
// the `result` event, then Maki exits (--exit-on-done).
//
// Session reuse: Maki's SDK mode honors `-s/--session` (unlike `--print`
// alone). The adapter looks up the prior session id for req.SessionKey in
// the SessionStore and adds `--session <sid>` to the invocation, so revise
// and review loops for the same role continue the same conversation
// context. The captured session id from each result event is written back
// to the store.
type MakiAdapter struct {
	bin  string
	sess SessionStore
}

// NewMakiAdapter builds a MakiAdapter. bin is the path to the `maki` binary.
// sessions is the session store; pass an in-memory or persistent
// implementation. nil sessions get an in-memory default.
func NewMakiAdapter(bin string, sessions SessionStore) *MakiAdapter {
	if bin == "" {
		bin = "maki"
	}
	if sessions == nil {
		sessions = newMemorySessionStore()
	}
	return &MakiAdapter{bin: bin, sess: sessions}
}

// BuildInvocation translates req into the Maki SDK-mode invocation. It adds
// `--session <sid>` when a prior session id exists for SessionKey.
func (a *MakiAdapter) BuildInvocation(_ context.Context, req Request) (Invocation, error) {
	if req.WorkingDir == "" {
		return Invocation{}, errors.New("agent: MakiAdapter: working dir is required")
	}
	args := []string{
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--exit-on-done",
	}
	if req.SessionKey != "" {
		if sid, ok := a.sess.Get(req.SessionKey); sid != "" && ok {
			args = append(args, "--session", sid)
		}
	}
	stdin, err := encodeUserMessage(req.Prompt)
	if err != nil {
		return Invocation{}, err
	}
	return Invocation{Bin: a.bin, Args: args, Dir: req.WorkingDir, Stdin: stdin}, nil
}

// ParseResult turns the captured stdout into a normalized Result. It scans
// the JSONL stream for the `result` event, captures the agent session id
// from any event wrapper, and writes it back to the SessionStore.
func (a *MakiAdapter) ParseResult(_ context.Context, req Request, raw RuntimeResult) (Result, error) {
	res := Result{IsError: raw.ExitErr != nil}

	if len(raw.Stdout) == 0 {
		res.IsError = true
		if raw.ExitErr != nil {
			return res, fmt.Errorf("agent: MakiAdapter: maki exited non-zero with no output: %w (stderr=%s)", raw.ExitErr, truncate(raw.Stderr))
		}
		return res, errors.New("agent: MakiAdapter: maki produced no stdout")
	}

	var (
		sessionID    string
		resultText   string
		resultSubtype string
		resultIsErr  bool
		foundResult  bool
	)

	for _, line := range splitLines(raw.Stdout) {
		var ev makiWireEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.SessionID != "" {
			sessionID = ev.SessionID
		}
		if ev.Type == "result" {
			foundResult = true
			resultText = ev.Result
			resultSubtype = ev.Subtype
			resultIsErr = ev.IsError || strings.EqualFold(ev.Subtype, "error")
		}
	}

	res.SessionID = sessionID
	res.Raw = resultText
	res.StopReason = resultSubtype

	if !foundResult {
		res.IsError = true
		return res, fmt.Errorf("agent: MakiAdapter: no result event in maki output (stderr=%s)", truncate(raw.Stderr))
	}
	if resultIsErr {
		res.IsError = true
	}

	if req.SessionKey != "" && sessionID != "" {
		a.sess.Put(req.SessionKey, sessionID)
	}

	return res, nil
}

// makiWireEvent mirrors the per-line shape of Maki's stream-json output.
// Every line carries a `session_id`; the `result` line carries the final
// outcome.
type makiWireEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
}

// encodeUserMessage wraps the prompt in the single inbound user message that
// Maki's SDK mode accepts on stdin.
func encodeUserMessage(prompt string) ([]byte, error) {
	if prompt == "" {
		return nil, errors.New("agent: MakiAdapter: prompt is required")
	}
	msg := map[string]any{
		"type":    "user",
		"message": map[string]any{"content": prompt},
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// splitLines splits b on newlines and drops empty trailing fragments. It
// does not strip other whitespace.
func splitLines(b []byte) [][]byte {
	var out [][]byte
	for _, line := range bytesSplit(b, '\n') {
		if len(line) > 0 {
			out = append(out, line)
		}
	}
	return out
}

// bytesSplit is a small allocation-light newline splitter.
func bytesSplit(b []byte, sep byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == sep {
			out = append(out, b[start:i])
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
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

// truncate returns the first max bytes of b with an ellipsis if truncated.
func truncate(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}