package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tzone85/px-dispatch/internal/state"
)

// resolveRepoDir picks the best candidate for the canonical repo directory:
//   1. the RepoPath of any non-archived requirement,
//   2. the parent of app.stateDir/worktrees (works when state lives under
//      <repo>/.px),
//   3. os.Getwd() as a last resort.
// Returns the empty string if none of these resolves to a real directory.
func resolveRepoDir() string {
	if app.projStore != nil {
		reqs, err := app.projStore.ListRequirements(state.ReqFilter{ExcludeArchived: true})
		if err == nil {
			for _, r := range reqs {
				if r.RepoPath != "" {
					if info, err := os.Stat(r.RepoPath); err == nil && info.IsDir() {
						return r.RepoPath
					}
				}
			}
		}
	}
	if app.stateDir != "" {
		candidate := filepath.Dir(app.stateDir)
		if info, err := os.Stat(filepath.Join(candidate, ".git")); err == nil {
			_ = info
			return candidate
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

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
