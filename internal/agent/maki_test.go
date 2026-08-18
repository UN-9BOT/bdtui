package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// fakeRunner is a goroutine-safe CommandRunner used by MakiAdapter tests.
type fakeRunner struct {
	mu      sync.Mutex
	calls   []fakeCall
	resp    fakeResponse
	respErr error
	dir     string // optional: recorded RunOpts.Dir of last call
}

type fakeCall struct {
	Bin  string
	Args []string
	Dir  string
}

type fakeResponse struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

func (f *fakeRunner) Run(_ context.Context, bin string, args []string, opts RunOpts) ([]byte, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{Bin: bin, Args: append([]string(nil), args...), Dir: opts.Dir})
	f.dir = opts.Dir
	return f.resp.Stdout, f.resp.Stderr, f.resp.Err
}

func (f *fakeRunner) lastCall() (fakeCall, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return fakeCall{}, false
	}
	return f.calls[len(f.calls)-1], true
}

func mkResultDir(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	result := filepath.Join(dir, "result.json")
	plan := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(result, []byte(`{"outcome":"planned","plan":"step 1"}`), 0o600); err != nil {
		t.Fatalf("write result: %v", err)
	}
	if err := os.WriteFile(plan, []byte("# Plan\nstep 1"), 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return dir, result, plan
}

func mkRequest(workingDir, result, plan string) Request {
	return Request{
		SessionKey: "run-1/planner",
		Prompt:     "deterministic-prompt",
		WorkingDir: workingDir,
		OutputPaths: OutputPaths{
			Result:    result,
			Artifacts: map[string]string{"plan": plan},
		},
	}
}

func TestMakiAdapterRunSuccess(t *testing.T) {
	_, result, plan := mkResultDir(t)
	dir := t.TempDir()

	fr := &fakeRunner{
		resp: fakeResponse{
			Stdout: []byte(`{"type":"result","subtype":"success","is_error":false,"stop_reason":"end_turn","result":"ok","session_id":"sid-abc"}`),
		},
	}
	a := NewMakiAdapter(fr, "maki", nil)
	res, err := a.Run(context.Background(), mkRequest(dir, result, plan))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %+v", res)
	}
	if res.SessionID != "sid-abc" {
		t.Fatalf("SessionID=%q", res.SessionID)
	}
	if string(res.ResultJSON) != `{"outcome":"planned","plan":"step 1"}` {
		t.Fatalf("ResultJSON=%q", res.ResultJSON)
	}
	if string(res.Artifacts["plan"]) != "# Plan\nstep 1" {
		t.Fatalf("Artifacts[plan]=%q", res.Artifacts["plan"])
	}

	call, ok := fr.lastCall()
	if !ok {
		t.Fatal("runner not called")
	}
	if call.Bin != "maki" {
		t.Fatalf("bin=%q", call.Bin)
	}
	wantArgs := []string{"--print", "--output-format", "json", "--exit-on-done", "deterministic-prompt"}
	if !equalStrings(call.Args, wantArgs) {
		t.Fatalf("args=%v want %v", call.Args, wantArgs)
	}
	if call.Dir != dir {
		t.Fatalf("Dir=%q want %q", call.Dir, dir)
	}
}

func TestMakiAdapterSessionStorePut(t *testing.T) {
	_, result, plan := mkResultDir(t)
	dir := t.TempDir()
	store := newMemorySessionStore()
	fr := &fakeRunner{
		resp: fakeResponse{Stdout: []byte(`{"session_id":"sid-1","is_error":false,"stop_reason":"end_turn","result":"ok"}`)},
	}
	a := NewMakiAdapter(fr, "maki", store)

	req := mkRequest(dir, result, plan)
	req.SessionKey = "run-42/planner"
	if _, err := a.Run(context.Background(), req); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, ok := store.Get("run-42/planner")
	if !ok || got != "sid-1" {
		t.Fatalf("store: got=%q ok=%v", got, ok)
	}
}

func TestMakiAdapterRunMalformedJSON(t *testing.T) {
	_, result, plan := mkResultDir(t)
	fr := &fakeRunner{resp: fakeResponse{Stdout: []byte(`{not-json`)}}
	a := NewMakiAdapter(fr, "maki", nil)
	res, err := a.Run(context.Background(), mkRequest(t.TempDir(), result, plan))
	if err == nil {
		t.Fatal("expected error")
	}
	if !res.IsError {
		t.Fatal("expected IsError")
	}
}

func TestMakiAdapterRunIsErrorFromMaki(t *testing.T) {
	_, result, plan := mkResultDir(t)
	fr := &fakeRunner{
		resp: fakeResponse{
			Stdout: []byte(`{"type":"result","subtype":"error","is_error":true,"stop_reason":"","result":"oops","session_id":"sid-e"}`),
		},
	}
	a := NewMakiAdapter(fr, "maki", nil)
	res, err := a.Run(context.Background(), mkRequest(t.TempDir(), result, plan))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError")
	}
	if res.SessionID != "sid-e" {
		t.Fatalf("SessionID=%q", res.SessionID)
	}
	if res.Raw != "oops" {
		t.Fatalf("Raw=%q", res.Raw)
	}
}

func TestMakiAdapterRunNonZeroExitOverridesSuccess(t *testing.T) {
	_, result, plan := mkResultDir(t)
	fr := &fakeRunner{
		resp: fakeResponse{
			Stdout: []byte(`{"is_error":false,"result":"x","session_id":"sid"}`),
			Err:    context.Canceled,
		},
	}
	a := NewMakiAdapter(fr, "maki", nil)
	res, err := a.Run(context.Background(), mkRequest(t.TempDir(), result, plan))
	if err == nil {
		t.Fatal("expected error")
	}
	if !res.IsError {
		t.Fatal("expected IsError after non-zero exit")
	}
}

func TestMakiAdapterRunEmptyOutput(t *testing.T) {
	_, result, plan := mkResultDir(t)
	fr := &fakeRunner{}
	a := NewMakiAdapter(fr, "maki", nil)
	res, err := a.Run(context.Background(), mkRequest(t.TempDir(), result, plan))
	if err == nil {
		t.Fatal("expected error")
	}
	if !res.IsError {
		t.Fatal("expected IsError")
	}
}

func TestMakiAdapterRequiresFields(t *testing.T) {
	a := NewMakiAdapter(&fakeRunner{}, "maki", nil)
	_, result, _ := mkResultDir(t)

	if _, err := a.Run(context.Background(), Request{WorkingDir: "/tmp", OutputPaths: OutputPaths{Result: result}}); err == nil {
		t.Fatal("expected error when prompt missing")
	}
	if _, err := a.Run(context.Background(), Request{Prompt: "p", OutputPaths: OutputPaths{Result: result}}); err == nil {
		t.Fatal("expected error when WorkingDir missing")
	}
	if _, err := a.Run(context.Background(), Request{Prompt: "p", WorkingDir: "/tmp"}); err == nil {
		t.Fatal("expected error when OutputPaths.Result missing")
	}
}

func TestMakiAdapterMissingArtifactNotFatal(t *testing.T) {
	// result.json present, plan artifact absent: Run still succeeds (the
	// completion check, not the adapter, gates on artifact presence).
	dir := t.TempDir()
	result := filepath.Join(dir, "result.json")
	if err := os.WriteFile(result, []byte(`{"outcome":"planned"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{
		resp: fakeResponse{Stdout: []byte(`{"is_error":false,"result":"x","session_id":"s"}`)},
	}
	a := NewMakiAdapter(fr, "maki", nil)
	res, err := a.Run(context.Background(), Request{
		SessionKey: "k",
		Prompt:     "p",
		WorkingDir: dir,
		OutputPaths: OutputPaths{
			Result:    result,
			Artifacts: map[string]string{"plan": filepath.Join(dir, "no-plan.md")},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Artifacts) != 0 {
		t.Fatalf("expected no artifacts, got %d", len(res.Artifacts))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
