package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMakiAdapterBuildInvocationFreshSession(t *testing.T) {
	a := NewMakiAdapter("maki", newMemorySessionStore())
	req := Request{
		SessionKey: "run-1/planner",
		Prompt:     "hello",
		WorkingDir: "/tmp",
	}
	inv, err := a.BuildInvocation(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildInvocation: %v", err)
	}
	if inv.Bin != "maki" {
		t.Fatalf("Bin=%q", inv.Bin)
	}
	want := []string{
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--exit-on-done",
	}
	if !equalStrings(inv.Args, want) {
		t.Fatalf("args=%v want %v", inv.Args, want)
	}
	if inv.Dir != "/tmp" {
		t.Fatalf("Dir=%q", inv.Dir)
	}
	stdin := string(inv.Stdin)
	if !strings.HasSuffix(stdin, "\n") {
		t.Fatalf("stdin should end end newline\n---%q---", stdin)
	}
	// Map key order in encoding/json is alphabetical, so do not assert prefix.
	var msg struct {
		Type    string `json:"type"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(stdin[:len(stdin)-1]), &msg); err != nil {
		t.Fatalf("stdin not valid JSON: %v\n---%s---", err, stdin)
	}
	if msg.Type != "user" || msg.Message.Content != "hello" {
		t.Fatalf("stdin fields wrong: %+v", msg)
	}
}

func TestMakiAdapterBuildInvocationResumeSession(t *testing.T) {
	store := newMemorySessionStore()
	store.Put("run-1/planner", "sid-existing")
	a := NewMakiAdapter("maki", store)
	inv, err := a.BuildInvocation(context.Background(), Request{
		SessionKey: "run-1/planner",
		Prompt:     "again",
		WorkingDir: "/tmp",
	})
	if err != nil {
		t.Fatalf("BuildInvocation: %v", err)
	}
	want := []string{
		"--print",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--exit-on-done",
		"--session", "sid-existing",
	}
	if !equalStrings(inv.Args, want) {
		t.Fatalf("args=%v want %v", inv.Args, want)
	}
}

func TestMakiAdapterBuildInvocationRequiresFields(t *testing.T) {
	a := NewMakiAdapter("maki", nil)
	if _, err := a.BuildInvocation(context.Background(), Request{Prompt: "p"}); err == nil {
		t.Fatal("expected error when WorkingDir missing")
	}
	if _, err := a.BuildInvocation(context.Background(), Request{WorkingDir: "/tmp"}); err == nil {
		t.Fatal("expected error when prompt missing")
	}
}

func TestMakiAdapterParseResultSuccess(t *testing.T) {
	a := NewMakiAdapter("maki", newMemorySessionStore())
	stdout := []byte(
		`{"type":"system","subtype":"init","session_id":"sid-1","cwd":"/repo","tools":["Bash"]}` + "\n" +
			`{"type":"assistant","message":{"id":"m1","model":"x","role":"assistant","content":"thinking","usage":{}},"session_id":"sid-1"}` + "\n" +
			`{"type":"result","subtype":"success","is_error":false,"result":"the agent reply","duration_ms":100,"num_turns":1,"session_id":"sid-1","total_cost_usd":0,"usage":{}}` + "\n",
	)
	res, err := a.ParseResult(context.Background(), Request{SessionKey: "k"}, RuntimeResult{Stdout: stdout})
	if err != nil {
		t.Fatalf("ParseResult: %v", err)
	}
	if res.IsError {
		t.Fatal("expected IsError=false")
	}
	if res.SessionID != "sid-1" {
		t.Fatalf("SessionID=%q", res.SessionID)
	}
	if res.Raw != "the agent reply" {
		t.Fatalf("Raw=%q", res.Raw)
	}
	if res.StopReason != "success" {
		t.Fatalf("StopReason=%q", res.StopReason)
	}
}

func TestMakiAdapterParseResultIsError(t *testing.T) {
	a := NewMakiAdapter("maki", nil)
	stdout := []byte(`{"type":"result","subtype":"error","is_error":true,"result":"oops","session_id":"sid-e"}` + "\n")
	res, err := a.ParseResult(context.Background(), Request{}, RuntimeResult{Stdout: stdout})
	if err != nil {
		t.Fatalf("ParseResult: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true")
	}
}

func TestMakiAdapterParseResultMissingResultEvent(t *testing.T) {
	a := NewMakiAdapter("maki", nil)
	stdout := []byte(`{"type":"system","subtype":"init","session_id":"sid"}` + "\n")
	res, err := a.ParseResult(context.Background(), Request{}, RuntimeResult{Stdout: stdout})
	if err == nil {
		t.Fatal("expected error when no result event")
	}
	if !res.IsError {
		t.Fatal("expected IsError")
	}
}

func TestMakiAdapterParseResultEmptyStdout(t *testing.T) {
	a := NewMakiAdapter("maki", nil)
	res, err := a.ParseResult(context.Background(), Request{}, RuntimeResult{ExitErr: context.Canceled})
	if err == nil {
		t.Fatal("expected error")
	}
	if !res.IsError {
		t.Fatal("expected IsError")
	}
}

func TestMakiAdapterSessionStoreUpdated(t *testing.T) {
	store := newMemorySessionStore()
	a := NewMakiAdapter("maki", store)
	stdout := []byte(`{"type":"result","subtype":"success","is_error":false,"result":"x","session_id":"sid-new"}` + "\n")
	if _, err := a.ParseResult(context.Background(), Request{SessionKey: "run-1/planner"}, RuntimeResult{Stdout: stdout}); err != nil {
		t.Fatalf("ParseResult: %v", err)
	}
	got, ok := store.Get("run-1/planner")
	if !ok || got != "sid-new" {
		t.Fatalf("store: got=%q ok=%v", got, ok)
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