package tmux

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tzone85/px-dispatch/internal/git"
)

func contains(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

func TestAvailable_True(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("/usr/bin/tmux", nil)

	if !Available(mock) {
		t.Error("expected tmux to be available")
	}
}

func TestAvailable_False(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("not found"))

	if Available(mock) {
		t.Error("expected tmux to not be available")
	}
}

func TestCreateSession(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session")) // has-session fails (no existing session)
	mock.AddResponse("", nil)                       // new-session succeeds

	err := CreateSession(mock, "test-session", "/tmp/work", "echo hello")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	found := false
	for _, cmd := range mock.Commands {
		if cmd.Name == "tmux" && contains(cmd.Args, "new-session") {
			found = true
			if !contains(cmd.Args, "-d") {
				t.Error("expected detached flag -d")
			}
			if !contains(cmd.Args, "-s") {
				t.Error("expected -s flag for session name")
			}
			if !contains(cmd.Args, "-c") {
				t.Error("expected -c flag for working directory")
			}
		}
	}
	if !found {
		t.Error("expected tmux new-session command")
	}
}

func TestCreateSession_KillsExistingFirst(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil) // has-session succeeds (session exists)
	mock.AddResponse("", nil) // kill-session succeeds
	mock.AddResponse("", nil) // new-session succeeds

	err := CreateSession(mock, "test-session", "/tmp/work", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if len(mock.Commands) != 3 {
		t.Fatalf("expected 3 commands (has-session, kill-session, new-session), got %d", len(mock.Commands))
	}

	killCmd := mock.Commands[1]
	if killCmd.Name != "tmux" || !contains(killCmd.Args, "kill-session") {
		t.Errorf("expected kill-session command, got %s %v", killCmd.Name, killCmd.Args)
	}
}

func TestCreateSession_NoCommand(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session")) // has-session fails
	mock.AddResponse("", nil)                       // new-session succeeds

	err := CreateSession(mock, "test-session", "/tmp/work", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	newCmd := mock.Commands[1]
	// Without a command, args should not include the command string
	expectedArgs := strings.Join([]string{"new-session", "-d", "-s", "test-session", "-c", "/tmp/work"}, " ")
	actualArgs := strings.Join(newCmd.Args, " ")
	if actualArgs != expectedArgs {
		t.Errorf("expected args %q, got %q", expectedArgs, actualArgs)
	}
}

func TestKillSession(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)

	err := KillSession(mock, "test-session")
	if err != nil {
		t.Fatalf("kill session: %v", err)
	}

	if len(mock.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(mock.Commands))
	}

	cmd := mock.Commands[0]
	if cmd.Name != "tmux" {
		t.Errorf("expected tmux command, got %s", cmd.Name)
	}
	if !contains(cmd.Args, "kill-session") {
		t.Errorf("expected kill-session in args, got %v", cmd.Args)
	}
	if !contains(cmd.Args, "test-session") {
		t.Errorf("expected session name in args, got %v", cmd.Args)
	}
}

func TestKillSession_Error(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("session not found"))

	err := KillSession(mock, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSessionExists_True(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil) // has-session succeeds

	if !SessionExists(mock, "test-session") {
		t.Error("expected session to exist")
	}
}

func TestSessionExists_False(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no session"))

	if SessionExists(mock, "test-session") {
		t.Error("expected session to not exist")
	}
}

func TestListSessions(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("session1\nsession2\nsession3", nil)

	sessions, err := ListSessions(mock)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	if sessions[0] != "session1" || sessions[1] != "session2" || sessions[2] != "session3" {
		t.Errorf("unexpected sessions: %v", sessions)
	}
}

func TestListSessions_NoServer(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no server running on /tmp/tmux"))

	sessions, err := ListSessions(mock)
	if err != nil {
		t.Fatalf("should not error for no server: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessions_NoSessions(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no sessions"))

	sessions, err := ListSessions(mock)
	if err != nil {
		t.Fatalf("should not error for no sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessions_OtherError(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("permission denied"))

	_, err := ListSessions(mock)
	if err == nil {
		t.Fatal("expected error for permission denied")
	}
}

func TestSendKeys(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", nil)

	err := SendKeys(mock, "test-session", "Y")
	if err != nil {
		t.Fatalf("send keys: %v", err)
	}

	if len(mock.Commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(mock.Commands))
	}

	cmd := mock.Commands[0]
	if cmd.Name != "tmux" {
		t.Errorf("expected tmux, got %s", cmd.Name)
	}
	if !contains(cmd.Args, "send-keys") {
		t.Errorf("expected send-keys in args, got %v", cmd.Args)
	}
	if !contains(cmd.Args, "Y") {
		t.Errorf("expected 'Y' in args, got %v", cmd.Args)
	}
	if !contains(cmd.Args, "Enter") {
		t.Errorf("expected 'Enter' in args, got %v", cmd.Args)
	}
}

func TestSendKeys_Error(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("session not found"))

	err := SendKeys(mock, "nonexistent", "Y")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReadOutput(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("line1\nline2\nline3", nil)

	output, err := ReadOutput(mock, "test-session", 30)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if output != "line1\nline2\nline3" {
		t.Errorf("unexpected output: %q", output)
	}
}

func TestReadOutput_Error(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("", fmt.Errorf("no pane"))

	_, err := ReadOutput(mock, "test-session", 30)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestReadOutput_VerifiesLineCount(t *testing.T) {
	mock := git.NewMockRunner()
	mock.AddResponse("output", nil)

	_, err := ReadOutput(mock, "test-session", 50)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	cmd := mock.Commands[0]
	if !contains(cmd.Args, "-50") {
		t.Errorf("expected -50 in capture-pane args, got %v", cmd.Args)
	}
}
