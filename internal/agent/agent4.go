package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// Agent4 implements the Agent interface for the Software Engineer role.
// It reads a task prompt file, produces code via LLM, and writes files to disk.
type Agent4 struct {
	cwd            string
	taskDir        string
	taskFile       string
	changePlanFile string // optional change plan file path (empty = no plan)
	fileTree       string
	turnCount      int
}

// NewAgent4 creates a new Agent 4 (Software Engineer) with the given working
// directory, task directory, and task file path. It scans the file tree at
// construction time (static per session).
func NewAgent4(cwd, taskDir, taskFile, changePlanFile string) *Agent4 {
	tree, _ := ScanFileTree(cwd, 3)
	return &Agent4{
		cwd:            cwd,
		taskDir:        taskDir,
		taskFile:       taskFile,
		changePlanFile: changePlanFile,
		fileTree:       tree,
	}
}

// Number returns the agent's pipeline position.
func (a *Agent4) Number() int { return 4 }

// Name returns the human-readable agent name.
func (a *Agent4) Name() string { return "Software Engineer" }

// SystemPrompt returns the system prompt for Agent 4, including CWD, file tree,
// and the content of the task prompt file.
func (a *Agent4) SystemPrompt() string {
	taskContent := ""
	if a.taskFile != "" {
		data, err := os.ReadFile(a.taskFile)
		if err == nil {
			taskContent = string(data)
		}
	}
	changePlanContent := ""
	if a.changePlanFile != "" {
		data, err := os.ReadFile(a.changePlanFile)
		if err == nil {
			changePlanContent = string(data)
		}
	}
	return Agent4SystemPrompt(a.cwd, a.fileTree, taskContent, changePlanContent)
}

// HandleResponse processes an LLM response. It increments the turn counter and
// checks for code blocks. Returns ActionComplete if code blocks are found or
// max turns reached; otherwise returns ActionContinue for multi-turn conversation.
func (a *Agent4) HandleResponse(response string) Action {
	a.turnCount++
	blocks := ParseCodeBlocks(response)
	if len(blocks) > 0 {
		return Action{Type: ActionComplete}
	}
	return Action{Type: ActionContinue}
}

// TaskDir returns the configured task directory path.
func (a *Agent4) TaskDir() string {
	return a.taskDir
}

// WriteHandoffFile creates a handoff markdown file for Agent 5 (Tester).
// The handoff file is named by stripping .md from taskFileName and appending
// "-handoff.md". It contains sections for changes made, files modified/created,
// and test suggestions. Writes are routed through the sandbox.
func WriteHandoffFile(sb *sandbox.Sandbox, taskDir, taskFileName string, results []FileResult, response string) (string, error) {
	// Derive handoff filename
	base := strings.TrimSuffix(taskFileName, ".md")
	handoffName := base + "-handoff.md"
	handoffPath := filepath.Join(taskDir, handoffName)

	// Extract summary: first non-empty paragraph that isn't a code block
	summary := extractSummary(response)

	// Build file list
	var fileList strings.Builder
	if len(results) > 0 {
		for _, r := range results {
			fmt.Fprintf(&fileList, "- %s (%s)\n", r.Path, r.Action)
		}
	} else {
		fileList.WriteString("- No files written\n")
	}

	// Build test suggestions from file list
	var testSuggestions strings.Builder
	if len(results) > 0 {
		testSuggestions.WriteString("Test the following files:\n")
		for _, r := range results {
			fmt.Fprintf(&testSuggestions, "- %s\n", r.Path)
		}
	} else {
		testSuggestions.WriteString("- No specific test suggestions\n")
	}

	// Compose handoff content
	var content strings.Builder
	content.WriteString("# Handoff: Code Changes\n\n")
	content.WriteString("## Changes Made\n\n")
	content.WriteString(summary)
	content.WriteString("\n\n")
	content.WriteString("## Files Modified/Created\n\n")
	content.WriteString(fileList.String())
	content.WriteString("\n")
	content.WriteString("## Test Suggestions\n\n")
	content.WriteString(testSuggestions.String())

	// Compute sandbox-relative paths
	relTaskDir, err := filepath.Rel(sb.CWD(), taskDir)
	if err != nil {
		relTaskDir = taskDir
	}
	relHandoffPath, err := filepath.Rel(sb.CWD(), handoffPath)
	if err != nil {
		relHandoffPath = handoffPath
	}

	if err := sb.MkdirAll(relTaskDir, 0o755); err != nil {
		return "", fmt.Errorf("create task directory: %w", err)
	}

	if err := sb.WriteFile(relHandoffPath, []byte(content.String()), 0o644); err != nil {
		return "", fmt.Errorf("write handoff file: %w", err)
	}

	return handoffPath, nil
}

// extractSummary pulls the first non-code-block paragraph from the response,
// or the first 200 characters if no clear paragraph is found.
func extractSummary(response string) string {
	lines := strings.Split(response, "\n")
	var summary strings.Builder
	inFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Toggle fence state
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}

		if inFence {
			continue
		}

		// Skip empty lines before we have content
		if summary.Len() == 0 && trimmed == "" {
			continue
		}

		// Stop at empty line after collecting content (end of paragraph)
		if summary.Len() > 0 && trimmed == "" {
			break
		}

		if summary.Len() > 0 {
			summary.WriteByte(' ')
		}
		summary.WriteString(trimmed)
	}

	result := summary.String()
	if result == "" && len(response) > 0 {
		result = response
		if len(result) > 200 {
			result = result[:200]
		}
	}
	return result
}
