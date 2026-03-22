package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent4_ImplementsAgent(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "001-test.md")
	require.NoError(t, os.WriteFile(taskFile, []byte("# Task: test"), 0o644))

	a := NewAgent4(dir, dir, taskFile, "")
	var _ Agent = a // compile-time interface check
	assert.NotNil(t, a)
}

func TestAgent4_Number(t *testing.T) {
	a := &Agent4{}
	assert.Equal(t, 4, a.Number())
}

func TestAgent4_Name(t *testing.T) {
	a := &Agent4{}
	assert.Equal(t, "Software Engineer", a.Name())
}

func TestAgent4_SystemPrompt_ContainsContext(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "001-add-retry.md")
	taskContent := "# Task: Add retry logic\n\n## Description\nAdd retry to HTTP client."
	require.NoError(t, os.WriteFile(taskFile, []byte(taskContent), 0o644))

	a := NewAgent4(dir, dir, taskFile, "")
	prompt := a.SystemPrompt()

	assert.Contains(t, prompt, dir)
	assert.Contains(t, prompt, "Add retry logic")
	assert.Contains(t, prompt, "Software Engineer")
}

func TestAgent4_HandleResponse_WithCodeBlocks(t *testing.T) {
	a := &Agent4{}
	response := "Here is the code:\n```go:main.go\npackage main\n```"
	action := a.HandleResponse(response)
	assert.Equal(t, ActionComplete, action.Type)
}

func TestAgent4_HandleResponse_NoCodeBlocks(t *testing.T) {
	a := &Agent4{}
	response := "I need more information about the project structure."
	action := a.HandleResponse(response)
	assert.Equal(t, ActionContinue, action.Type)
	assert.Equal(t, 1, a.turnCount)
}

func TestAgent4_HandleResponse_ContinuesWithoutBlocks(t *testing.T) {
	a := &Agent4{turnCount: 10}
	// No code blocks -- keeps going regardless of turn count
	response := "Still thinking about this..."
	action := a.HandleResponse(response)
	assert.Equal(t, ActionContinue, action.Type)
}

func TestWriteHandoffFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	results := []FileResult{
		{Path: "internal/server/client.go", Action: "modified"},
		{Path: "internal/server/retry.go", Action: "created"},
	}
	response := "I added retry logic to the HTTP client.\n\n```go:internal/server/client.go\npackage server\n```"

	path, err := WriteHandoffFile(sb, taskDir, "001-add-retry.md", results, response)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(taskDir, "001-add-retry-handoff.md"), path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(content)
	assert.Contains(t, text, "Changes Made")
	assert.Contains(t, text, "Files Modified/Created")
	assert.Contains(t, text, "internal/server/client.go")
	assert.Contains(t, text, "internal/server/retry.go")
	assert.Contains(t, text, "Test Suggestions")
}

func TestAgent4_SystemPromptWithChangePlan(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "001-add-retry.md")
	taskContent := "# Task: Add retry logic"
	require.NoError(t, os.WriteFile(taskFile, []byte(taskContent), 0o644))

	changePlanFile := filepath.Join(dir, "001-add-retry-change-plan.md")
	changePlanContent := "# Change Plan: Add retry\n\n## Changes\n### MODIFY internal/client.go\n- MODIFY func DoRequest: add retry loop"
	require.NoError(t, os.WriteFile(changePlanFile, []byte(changePlanContent), 0o644))

	a := NewAgent4(dir, dir, taskFile, changePlanFile)
	prompt := a.SystemPrompt()

	assert.Contains(t, prompt, "Change Plan (from Software Architect)")
	assert.Contains(t, prompt, "MODIFY internal/client.go")
	assert.Contains(t, prompt, "MODIFY func DoRequest")
}

func TestAgent4_SystemPromptWithMissingChangePlan(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "001-add-retry.md")
	require.NoError(t, os.WriteFile(taskFile, []byte("# Task: test"), 0o644))

	// Point to a non-existent file -- should gracefully omit change plan section
	a := NewAgent4(dir, dir, taskFile, "/nonexistent/change-plan.md")
	prompt := a.SystemPrompt()

	assert.NotContains(t, prompt, "Change Plan (from Software Architect)")
}

func TestAgent4_SystemPromptBackwardCompat(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "001-add-retry.md")
	taskContent := "# Task: Add retry logic"
	require.NoError(t, os.WriteFile(taskFile, []byte(taskContent), 0o644))

	a := NewAgent4(dir, dir, taskFile, "")
	prompt := a.SystemPrompt()

	assert.NotContains(t, prompt, "Change Plan (from Software Architect)")
	assert.Contains(t, prompt, "Add retry logic")
	assert.Contains(t, prompt, "Software Engineer")
}

func TestAgent4SystemPrompt_WithChangePlan(t *testing.T) {
	result := Agent4SystemPrompt("cwd", "tree", "task content", "some plan")
	assert.Contains(t, result, "Change Plan (from Software Architect)")
	assert.Contains(t, result, "some plan")
}

func TestAgent4SystemPrompt_WithoutChangePlan(t *testing.T) {
	result := Agent4SystemPrompt("cwd", "tree", "task content", "")
	assert.NotContains(t, result, "Change Plan (from Software Architect)")
	assert.Contains(t, result, "task content")
}

func TestWriteHandoffFile_HandoffNaming(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteHandoffFile(sb, taskDir, "042-fix-bug.md", nil, "Fixed the bug.")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(taskDir, "042-fix-bug-handoff.md"), path)
}
