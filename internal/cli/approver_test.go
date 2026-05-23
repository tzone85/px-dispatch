package cli

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/tzone85/px-dispatch/internal/modelswitch"
)

func TestNewCLIModelSwitchApprover_InitMap(t *testing.T) {
	a := newCLIModelSwitchApprover()
	if a == nil {
		t.Fatal("nil approver")
	}
	if a.decisions == nil {
		t.Error("decisions map not initialized")
	}
}

func TestApproveSwitch_CachedDecision(t *testing.T) {
	a := newCLIModelSwitchApprover()
	req := modelswitch.Request{
		Scope:          modelswitch.ScopeLLM,
		TargetProvider: "openai",
		TargetRuntime:  "codex",
		TargetModel:    "gpt-4o",
	}
	a.decisions[approvalKey(req)] = true

	ok, err := a.ApproveSwitch(req)
	if err != nil {
		t.Fatalf("ApproveSwitch returned err: %v", err)
	}
	if !ok {
		t.Error("expected cached true decision")
	}
}

func TestApproveSwitch_CachedFalse(t *testing.T) {
	a := newCLIModelSwitchApprover()
	req := modelswitch.Request{Scope: modelswitch.ScopeRuntime, TargetRuntime: "gemini"}
	a.decisions[approvalKey(req)] = false

	ok, err := a.ApproveSwitch(req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Error("expected cached false decision")
	}
}

func TestApproveSwitch_NoTTYRejected(t *testing.T) {
	a := newCLIModelSwitchApprover()
	// Redirect stdin so it's not a TTY and replace /dev/tty path implicitly:
	// in CI / non-TTY environments os.Open("/dev/tty") fails and we hit the
	// error branch in openApprovalInput → ApproveSwitch returns wrapped error.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		w.Close()
	})

	// If we're running in an environment where /dev/tty IS openable, the
	// approver may instead attempt to read from it. To avoid blocking, write
	// "n" to the read end so the prompt gets an answer.
	_, _ = w.WriteString("n\n")
	w.Close()

	req := modelswitch.Request{
		Scope:          modelswitch.ScopeLLM,
		TargetProvider: "openai",
		TargetRuntime:  "codex",
		TargetModel:    "gpt-4o",
		Reason:         "claude died",
	}
	out := captureStdout(t, func() {
		_, _ = a.ApproveSwitch(req)
	})
	_ = out // we don't assert content; just exercising the path
}

// withStubApprovalInput replaces the approver's input source with a pipe pre-
// loaded with `response`. Tests can therefore deterministically exercise the
// approval path without depending on a real TTY.
func withStubApprovalInput(t *testing.T, response string) {
	t.Helper()
	prev := approvalInputOpener
	approvalInputOpener = func() (*os.File, *bufio.Reader, error) {
		r, w, err := os.Pipe()
		if err != nil {
			return nil, nil, err
		}
		_, _ = w.WriteString(response)
		w.Close()
		return r, bufio.NewReader(r), nil
	}
	t.Cleanup(func() { approvalInputOpener = prev })
}

func TestApproveSwitch_AcceptsWhenInputIsYes(t *testing.T) {
	withStubApprovalInput(t, "y\n")

	a := newCLIModelSwitchApprover()
	req := modelswitch.Request{
		Scope:          modelswitch.ScopeLLM,
		TargetProvider: "openai",
		TargetRuntime:  "codex",
		TargetModel:    "gpt-4o",
		Operation:      "fallback",
		StoryID:        "S-1",
		StoryTitle:     "do thing",
		Reason:         "claude exhausted",
		Note:           "approved hand-off",
	}

	var ok bool
	var err error
	out := captureStdout(t, func() {
		ok, err = a.ApproveSwitch(req)
	})
	if err != nil {
		t.Fatalf("ApproveSwitch: %v", err)
	}
	if !ok {
		t.Error("expected approval=true")
	}
	if !strings.Contains(out, "Approve this switch") {
		t.Errorf("expected approval prompt in output, got %q", out)
	}
	if !strings.Contains(out, "do thing") {
		t.Errorf("expected story title in prompt, got %q", out)
	}
	if !strings.Contains(out, "fallback") {
		t.Errorf("expected operation in prompt, got %q", out)
	}
	if !strings.Contains(out, "approved hand-off") {
		t.Errorf("expected note in prompt, got %q", out)
	}
	// And cached.
	if a.decisions[approvalKey(req)] != true {
		t.Error("decision should be cached as true")
	}
}

func TestApproveSwitch_RejectsWhenInputIsNo(t *testing.T) {
	withStubApprovalInput(t, "n\n")
	a := newCLIModelSwitchApprover()
	req := modelswitch.Request{
		Scope:         modelswitch.ScopeRuntime,
		TargetRuntime: "gemini",
		Reason:        "denied",
	}

	captureStdout(t, func() {
		ok, err := a.ApproveSwitch(req)
		if err != nil {
			t.Fatalf("ApproveSwitch: %v", err)
		}
		if ok {
			t.Error("expected approval=false")
		}
	})
}

func TestApproveSwitch_PromptOmitsOptionalFields(t *testing.T) {
	withStubApprovalInput(t, "n\n")
	a := newCLIModelSwitchApprover()
	req := modelswitch.Request{
		Scope:         modelswitch.ScopeRuntime,
		TargetRuntime: "gemini",
		Reason:        "x",
		// Operation, StoryID, StoryTitle, Note, TargetProvider all empty.
	}
	out := captureStdout(t, func() {
		_, _ = a.ApproveSwitch(req)
	})
	if strings.Contains(out, "Operation:") {
		t.Errorf("Operation header should be omitted when empty: %q", out)
	}
	if strings.Contains(out, "Story:") {
		t.Errorf("Story header should be omitted when empty: %q", out)
	}
	if strings.Contains(out, "Note:") {
		t.Errorf("Note header should be omitted when empty: %q", out)
	}
}

func TestApproveSwitch_InputOpenError(t *testing.T) {
	prev := approvalInputOpener
	approvalInputOpener = func() (*os.File, *bufio.Reader, error) {
		return nil, nil, errors.New("no tty")
	}
	t.Cleanup(func() { approvalInputOpener = prev })

	a := newCLIModelSwitchApprover()
	req := modelswitch.Request{Scope: modelswitch.ScopeLLM, TargetRuntime: "codex", TargetModel: "gpt-4o"}
	if _, err := a.ApproveSwitch(req); err == nil {
		t.Error("expected error when input opener fails")
	}
}

func TestApproveSwitch_ReadError(t *testing.T) {
	prev := approvalInputOpener
	approvalInputOpener = func() (*os.File, *bufio.Reader, error) {
		// Pipe with empty content + immediate close → ReadString returns io.EOF.
		r, w, _ := os.Pipe()
		w.Close()
		return r, bufio.NewReader(r), nil
	}
	t.Cleanup(func() { approvalInputOpener = prev })

	a := newCLIModelSwitchApprover()
	req := modelswitch.Request{Scope: modelswitch.ScopeRuntime, TargetRuntime: "codex"}
	captureStdout(t, func() {
		_, err := a.ApproveSwitch(req)
		if err == nil {
			t.Error("expected error from empty input")
		}
	})
}

func TestStdinIsInteractive_ClosedFD_ReturnsFalse(t *testing.T) {
	// A closed file descriptor's Stat() should produce an error and
	// stdinIsInteractive must return false.
	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	r.Close()
	w.Close()

	if stdinIsInteractive() {
		t.Error("closed stdin should not report as interactive")
	}
}

func TestStdinIsInteractive_Pipe(t *testing.T) {
	// When stdin is a pipe (as it is under test runners), stdinIsInteractive
	// should return false.
	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		w.Close()
	})
	if stdinIsInteractive() {
		t.Error("piped stdin should not report as interactive")
	}
}

func TestOpenApprovalInput_PipeFallsBackToTTYOrErrors(t *testing.T) {
	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		w.Close()
	})

	f, reader, err := openApprovalInput()
	if err != nil {
		// Acceptable: no /dev/tty in this environment.
		return
	}
	// If /dev/tty IS available, close it to avoid leaking the fd.
	if f != os.Stdin {
		_ = f.Close()
	}
	if reader == nil {
		t.Error("reader should be non-nil on success")
	}
}
