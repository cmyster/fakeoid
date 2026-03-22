package agent

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// FileContent holds a file path and its content for token budget calculations.
type FileContent struct {
	Path    string
	Content string
}

// ParseReadBlocks extracts file paths from ```read fenced blocks in an LLM response.
// Only blocks starting with exactly "```read" are matched; annotated blocks like
// "```go:path/to/file.go" are NOT matched. Paths are trimmed; empty lines are skipped.
func ParseReadBlocks(response string) []string {
	var paths []string
	scanner := bufio.NewScanner(strings.NewReader(response))
	inRead := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if inRead {
			// Closing fence
			if strings.HasPrefix(trimmed, "```") {
				inRead = false
				continue
			}
			// Non-empty trimmed lines are paths
			if trimmed != "" {
				paths = append(paths, trimmed)
			}
			continue
		}

		// Check for opening ```read fence (must be exactly "```read", not "```go:path")
		if trimmed == "```read" {
			inRead = true
		}
	}

	return paths
}

// EstimateTokens returns a rough token estimate using char/4 heuristic.
// Matches the pattern used by AgentRunner.TrimOldest().
func EstimateTokens(content string) int {
	return len(content) / 4
}

// TokenBudget computes the available token budget for file injection.
// It reserves 60% of the context window for file content, minus the estimated
// tokens for the system prompt and task content. Returns 0 if the result
// would be negative.
func TokenBudget(ctxSize int, systemPromptChars int, taskChars int) int {
	budget := ctxSize*60/100 - systemPromptChars/4 - taskChars/4
	if budget < 0 {
		return 0
	}
	return budget
}

// ApplyTokenBudget selects files that fit within the token budget.
// Files are considered in order; when adding a file would exceed the budget,
// that file and all remaining files are omitted. No mid-file truncation.
func ApplyTokenBudget(files []FileContent, budgetTokens int) (kept []FileContent, omitted []string) {
	usedTokens := 0
	for i, f := range files {
		tokens := EstimateTokens(f.Content)
		if usedTokens+tokens > budgetTokens {
			// This file and all remaining go to omitted
			for _, rem := range files[i:] {
				omitted = append(omitted, rem.Path)
			}
			return kept, omitted
		}
		usedTokens += tokens
		kept = append(kept, f)
	}
	return kept, omitted
}

// WriteEnrichedFile creates an enriched prompt file through the sandbox.
// The file is named by stripping .md from taskFileName and appending
// "-enriched.md". Returns the absolute path to the created file.
func WriteEnrichedFile(sb *sandbox.Sandbox, taskDir, taskFileName, content string) (string, error) {
	base := strings.TrimSuffix(taskFileName, ".md")
	enrichedName := base + "-enriched.md"
	enrichedPath := filepath.Join(taskDir, enrichedName)

	relTaskDir, err := filepath.Rel(sb.CWD(), taskDir)
	if err != nil {
		relTaskDir = taskDir
	}
	relEnrichedPath, err := filepath.Rel(sb.CWD(), enrichedPath)
	if err != nil {
		relEnrichedPath = enrichedPath
	}

	if err := sb.MkdirAll(relTaskDir, 0o755); err != nil {
		return "", fmt.Errorf("create task directory: %w", err)
	}
	if err := sb.WriteFile(relEnrichedPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write enriched file: %w", err)
	}
	return enrichedPath, nil
}
