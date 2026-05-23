package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/tzone85/px-dispatch/internal/modelswitch"
)

var modelSwitchApprover = newCLIModelSwitchApprover()

type cliModelSwitchApprover struct {
	mu        sync.Mutex
	decisions map[string]bool
}

func newCLIModelSwitchApprover() *cliModelSwitchApprover {
	return &cliModelSwitchApprover{
		decisions: make(map[string]bool),
	}
}

func (a *cliModelSwitchApprover) ApproveSwitch(req modelswitch.Request) (bool, error) {
	key := approvalKey(req)

	a.mu.Lock()
	defer a.mu.Unlock()

	if decision, ok := a.decisions[key]; ok {
		return decision, nil
	}

	inputFile, reader, err := openApprovalInput()
	if err != nil {
		return false, fmt.Errorf(
			"approval required to switch to %s/%s, but no interactive terminal is available: %w",
			req.TargetRuntime, req.TargetModel, err,
		)
	}
	if inputFile != os.Stdin {
		defer inputFile.Close()
	}

	fmt.Println()
	fmt.Println("Claude can no longer continue this run.")
	if req.Operation != "" {
		fmt.Printf("Operation: %s\n", req.Operation)
	}
	if req.StoryID != "" {
		fmt.Printf("Story: %s", req.StoryID)
		if req.StoryTitle != "" {
			fmt.Printf(" (%s)", req.StoryTitle)
		}
		fmt.Println()
	}
	fmt.Printf("Reason: %s\n", req.Reason)
	fmt.Printf("Fallback: %s/%s", req.TargetRuntime, req.TargetModel)
	if req.TargetProvider != "" {
		fmt.Printf(" via %s", req.TargetProvider)
	}
	fmt.Println()
	if req.Note != "" {
		fmt.Printf("Note: %s\n", req.Note)
	}
	fmt.Print("Approve this switch for the remainder of this command? [y/N]: ")

	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, err
	}

	approved := parseApproval(line)
	a.decisions[key] = approved
	return approved, nil
}

func approvalKey(req modelswitch.Request) string {
	return strings.Join([]string{
		string(req.Scope),
		req.TargetProvider,
		req.TargetRuntime,
		req.TargetModel,
	}, "|")
}

func stdinIsInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// approvalInputOpener returns the file + reader used to gather an approval
// decision. Tests override this to avoid depending on a real TTY.
var approvalInputOpener = defaultOpenApprovalInput

func openApprovalInput() (*os.File, *bufio.Reader, error) {
	return approvalInputOpener()
}

func defaultOpenApprovalInput() (*os.File, *bufio.Reader, error) {
	// /dev/tty works both when stdin is interactive (returns the same TTY) and
	// when stdin is piped (provides a fallback channel for the prompt).
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return nil, nil, err
	}
	return tty, bufio.NewReader(tty), nil
}

func parseApproval(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
