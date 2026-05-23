package monitor

import (
	"os"
	"path/filepath"
	"strings"
)

// detectExistingCodebase reports whether the worktree at path looks like an
// existing project the agent has to respect — i.e. it has tracked source files
// beyond a basic skeleton. We treat anything with >1 source file or an
// existing README/CLAUDE.md as "existing".
func detectExistingCodebase(worktreePath string) bool {
	if worktreePath == "" {
		return false
	}
	// Quick wins: presence of a real README or AI agent instructions strongly
	// implies an existing repo.
	for _, marker := range []string{"README.md", "CLAUDE.md", "AGENTS.md", "GEMINI.md"} {
		if info, err := os.Stat(filepath.Join(worktreePath, marker)); err == nil && info.Size() > 64 {
			return true
		}
	}
	// Otherwise count tracked source files (any common language).
	sourceExts := map[string]bool{
		".go": true, ".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".rs": true, ".java": true, ".kt": true, ".rb": true, ".c": true, ".cpp": true,
		".cs": true, ".php": true, ".swift": true, ".vue": true, ".svelte": true,
	}
	count := 0
	_ = filepath.WalkDir(worktreePath, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if sourceExts[strings.ToLower(filepath.Ext(p))] {
			count++
			if count >= 2 {
				return filepath.SkipAll
			}
		}
		return nil
	})
	return count >= 2
}

// classifyIsBugFix uses keyword heuristics on the story title + description to
// flag bug-fix work. Triggers the bug-hunting methodology in the goal prompt.
var bugFixKeywords = []string{
	"fix ", "bug", "regression", "broken", "incorrect", "crash", "panic",
	"error in", "issue", "defect", "patch ", "hotfix",
}

func classifyIsBugFix(title, description string) bool {
	hay := strings.ToLower(title + " " + description)
	for _, kw := range bugFixKeywords {
		if strings.Contains(hay, kw) {
			return true
		}
	}
	return false
}

// classifyIsInfrastructure flags Docker/CI/deploy-shaped work so the agent
// gets the infrastructure debugging playbook.
var infraKeywords = []string{
	"docker", "kubernetes", "k8s", "compose", "helm",
	"ci/cd", "github actions", "gitlab ci", "jenkins", "circleci",
	"deploy", "deployment", "terraform", "ansible", "pulumi",
	"makefile", "infrastructure",
}

func classifyIsInfrastructure(title, description string, ownedFiles []string) bool {
	hay := strings.ToLower(title + " " + description)
	for _, kw := range infraKeywords {
		if strings.Contains(hay, kw) {
			return true
		}
	}
	for _, f := range ownedFiles {
		name := strings.ToLower(filepath.Base(f))
		if name == "dockerfile" || strings.HasPrefix(name, "docker-compose") ||
			strings.HasSuffix(name, ".tf") || strings.HasSuffix(name, ".yaml") && strings.Contains(f, ".github/workflows/") {
			return true
		}
	}
	return false
}

// loadWaveContext reads the optional WAVE_CONTEXT.md file from the worktree.
// Tech-lead or earlier waves can drop a summary of merged work here so later
// agents inherit context without re-reading every commit.
func (e *Executor) loadWaveContext(worktreePath string) string {
	if worktreePath == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(worktreePath, "WAVE_CONTEXT.md"))
	if err != nil {
		return ""
	}
	// Cap at 8 KB so we don't blow out the prompt with a huge context blob.
	const maxBytes = 8 * 1024
	if len(data) > maxBytes {
		data = data[:maxBytes]
	}
	return strings.TrimSpace(string(data))
}
