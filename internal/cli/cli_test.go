package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/tzone85/px-dispatch/internal/modelswitch"
	"github.com/tzone85/px-dispatch/internal/state"
)

func TestBudgetBar(t *testing.T) {
	tests := []struct {
		name  string
		used  float64
		limit float64
		width int
		want  string
	}{
		{"zero limit", 50, 0, 10, "[??] [??????????]"},
		{"negative limit", 50, -1, 10, "[??] [??????????]"},
		{"zero used", 0, 100, 10, "[ok] [..........]"},
		{"50 percent", 50, 100, 10, "[ok] [#####.....]"},
		{"60 percent", 60, 100, 10, "[! ] [######....]"},
		{"79 percent", 79, 100, 10, "[! ] [#######...]"},
		{"80 percent", 80, 100, 10, "[!!] [########..]"},
		{"100 percent", 100, 100, 10, "[!!] [##########]"},
		{"over 100 percent", 150, 100, 10, "[!!] [##########]"},
		{"small width", 50, 100, 4, "[ok] [##..]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := budgetBar(tt.used, tt.limit, tt.width)
			if got != tt.want {
				t.Errorf("budgetBar(%v, %v, %d) = %q, want %q", tt.used, tt.limit, tt.width, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{"short string", "abc", 10, "abc"},
		{"exact length", "abcde", 5, "abcde"},
		{"over length", "abcdefghij", 7, "abcd..."},
		{"n equals 3", "abcdef", 3, "abc"},
		{"n equals 2", "abcdef", 2, "ab"},
		{"n equals 1", "abcdef", 1, "a"},
		{"unicode", "héllo wörld", 8, "héllo..."},
		{"empty string", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

func TestCountByStatus(t *testing.T) {
	tests := []struct {
		name    string
		stories []state.Story
		want    map[string]int
	}{
		{"empty", nil, map[string]int{}},
		{
			"mixed",
			[]state.Story{
				{Status: "planned"},
				{Status: "assigned"},
				{Status: "planned"},
				{Status: "done"},
				{Status: "assigned"},
				{Status: "assigned"},
			},
			map[string]int{"planned": 2, "assigned": 3, "done": 1},
		},
		{
			"all same",
			[]state.Story{{Status: "done"}, {Status: "done"}},
			map[string]int{"done": 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countByStatus(tt.stories)
			if len(got) != len(tt.want) {
				t.Errorf("countByStatus() got %d statuses, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("countByStatus()[%q] = %d, want %d", k, got[k], v)
				}
			}
		})
	}
}

func TestParseApproval(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y", true},
		{"Y", true},
		{"yes", true},
		{"YES", true},
		{"Yes", true},
		{" y ", true},
		{" yes\n", true},
		{"n", false},
		{"no", false},
		{"N", false},
		{"", false},
		{"maybe", false},
		{"yep", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseApproval(tt.input)
			if got != tt.want {
				t.Errorf("parseApproval(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestApprovalKey(t *testing.T) {
	req := modelswitch.Request{
		Scope:          modelswitch.ScopeLLM,
		TargetProvider: "openai",
		TargetRuntime:  "codex",
		TargetModel:    "gpt-4o",
	}

	got := approvalKey(req)
	want := "llm|openai|codex|gpt-4o"
	if got != want {
		t.Errorf("approvalKey() = %q, want %q", got, want)
	}
}

func TestApprovalKey_EmptyFields(t *testing.T) {
	req := modelswitch.Request{Scope: modelswitch.ScopeRuntime}
	got := approvalKey(req)
	want := "runtime|||"
	if got != want {
		t.Errorf("approvalKey() = %q, want %q", got, want)
	}
}

func TestShouldSkipInit(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{"nil", nil, true},
		{"version", &cobra.Command{Use: "version"}, true},
		{"help", &cobra.Command{Use: "help"}, true},
		{"status", &cobra.Command{Use: "status"}, false},
		{"cost", &cobra.Command{Use: "cost"}, false},
		{"plan", &cobra.Command{Use: "plan"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipInit(tt.cmd)
			if got != tt.want {
				t.Errorf("shouldSkipInit(%v) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestNewRootCmd(t *testing.T) {
	cmd := NewRootCmd()
	if cmd == nil {
		t.Fatal("NewRootCmd returned nil")
	}
	if cmd.Use != "px" {
		t.Errorf("Use = %q, want %q", cmd.Use, "px")
	}

	// Verify key subcommands exist.
	expectedCmds := []string{"version", "status", "cost", "plan", "resume", "dashboard", "events", "migrate"}
	for _, name := range expectedCmds {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd()
	if cmd.Use != "version" {
		t.Errorf("Use = %q, want %q", cmd.Use, "version")
	}
}
