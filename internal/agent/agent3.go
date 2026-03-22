package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// Agent3 implements the Agent interface for the Software Architect role.
// It is a single-turn agent that receives task description + AST-extracted
// codebase structure and produces a markdown change plan.
type Agent3 struct {
	cwd         string
	taskDir     string
	taskFile    string
	fileTree    string
	astMarkdown string
}

// NewAgent3 creates a new Agent 3 (Software Architect) with the given working
// directory, task directory, task file path, and AST markdown. It scans the
// file tree at construction time (static per session).
func NewAgent3(cwd, taskDir, taskFile, astMarkdown string) *Agent3 {
	tree, _ := ScanFileTree(cwd, 3)
	return &Agent3{
		cwd:         cwd,
		taskDir:     taskDir,
		taskFile:    taskFile,
		fileTree:    tree,
		astMarkdown: astMarkdown,
	}
}

// Number returns the agent's pipeline position.
func (a *Agent3) Number() int { return 3 }

// Name returns the human-readable agent name.
func (a *Agent3) Name() string { return "Software Architect" }

// SystemPrompt returns the system prompt for Agent 3, including CWD, file tree,
// AST markdown, and the content of the task prompt file.
func (a *Agent3) SystemPrompt() string {
	taskContent := ""
	if a.taskFile != "" {
		data, err := os.ReadFile(a.taskFile)
		if err == nil {
			taskContent = string(data)
		}
	}
	return Agent3SystemPrompt(a.cwd, a.fileTree, a.astMarkdown, taskContent)
}

// HandleResponse processes an LLM response. Agent 3 is single-turn:
// it always returns ActionComplete after the first response.
func (a *Agent3) HandleResponse(response string) Action {
	return Action{Type: ActionComplete}
}

// TaskDir returns the configured task directory path.
func (a *Agent3) TaskDir() string {
	return a.taskDir
}

// WriteChangePlanFile creates a change plan markdown file for Agent 4.
// The file is named by stripping .md from taskFileName and appending
// "-change-plan.md". It contains the change plan content produced by Agent 3.
// Writes are routed through the sandbox.
func WriteChangePlanFile(sb *sandbox.Sandbox, taskDir, taskFileName, content string) (string, error) {
	base := strings.TrimSuffix(taskFileName, ".md")
	planName := base + "-change-plan.md"
	planPath := filepath.Join(taskDir, planName)

	relTaskDir, err := filepath.Rel(sb.CWD(), taskDir)
	if err != nil {
		relTaskDir = taskDir
	}
	relPlanPath, err := filepath.Rel(sb.CWD(), planPath)
	if err != nil {
		relPlanPath = planPath
	}

	if err := sb.MkdirAll(relTaskDir, 0o755); err != nil {
		return "", fmt.Errorf("create task directory: %w", err)
	}
	if err := sb.WriteFile(relPlanPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write change plan file: %w", err)
	}
	return planPath, nil
}
