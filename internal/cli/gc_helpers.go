package cli

import (
	"os/exec"
	"strings"
)

// listLocalPxBranches returns every local branch whose name starts with "px/".
func listLocalPxBranches(repoDir string) ([]string, error) {
	cmd := exec.Command("git", "branch", "--list", "px/*", "--format=%(refname:short)")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

// runGit runs a git command quietly in repoDir.
func runGit(repoDir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	return cmd.Run()
}

// runShellCapture runs an external command and returns its combined output.
func runShellCapture(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// runShellQuiet runs an external command, discarding output.
func runShellQuiet(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}
