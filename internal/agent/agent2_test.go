package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent2_ImplementsAgent(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "001-test.md")
	require.NoError(t, os.WriteFile(taskFile, []byte("# Task: test"), 0o644))

	a := NewAgent2(dir, dir, taskFile, 8192)
	var _ Agent = a // compile-time interface check
	assert.NotNil(t, a)
}

func TestNewAgent2_Fields(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "001-test.md")
	require.NoError(t, os.WriteFile(taskFile, []byte("# Task: test"), 0o644))

	a := NewAgent2(dir, dir, taskFile, 8192)
	assert.Equal(t, 8192*60/100, a.tokenBudget)
	assert.NotNil(t, a.readFiles)
	assert.NotEmpty(t, a.fileTree)
}

func TestAgent2_Number(t *testing.T) {
	a := &Agent2{}
	assert.Equal(t, 2, a.Number())
}

func TestAgent2_Name(t *testing.T) {
	a := &Agent2{}
	assert.Equal(t, "Prompt Engineer", a.Name())
}

func TestAgent2_SystemPrompt_ContainsContext(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "001-add-retry.md")
	taskContent := "# Task: Add retry logic\n\n## Description\nAdd retry to HTTP client."
	require.NoError(t, os.WriteFile(taskFile, []byte(taskContent), 0o644))

	a := NewAgent2(dir, dir, taskFile, 8192)
	prompt := a.SystemPrompt()

	assert.Contains(t, prompt, dir)
	assert.Contains(t, prompt, "Add retry logic")
	assert.Contains(t, prompt, "Prompt Engineer")
	assert.Contains(t, prompt, "## Goal")
	assert.Contains(t, prompt, "## Context")
	assert.Contains(t, prompt, "## File Tree")
	assert.Contains(t, prompt, "## Constraints")
	assert.Contains(t, prompt, "## Affected Files")
}

func TestAgent2_SystemPrompt_ContainsFileTree(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory so file tree is non-empty
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "main.go"), []byte("package main"), 0o644))
	taskFile := filepath.Join(dir, "task.md")
	require.NoError(t, os.WriteFile(taskFile, []byte("# Task"), 0o644))

	a := NewAgent2(dir, dir, taskFile, 8192)
	prompt := a.SystemPrompt()

	assert.Contains(t, prompt, "internal/")
}

func TestAgent2_SystemPrompt_ContainsReadInstructions(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "task.md")
	require.NoError(t, os.WriteFile(taskFile, []byte("# Task"), 0o644))

	a := NewAgent2(dir, dir, taskFile, 8192)
	prompt := a.SystemPrompt()

	assert.Contains(t, prompt, "```read")
}

func TestAgent2_SystemPrompt_EmptyTaskFile(t *testing.T) {
	dir := t.TempDir()
	a := NewAgent2(dir, dir, "", 8192)
	prompt := a.SystemPrompt()

	// Should not panic, just produce prompt without task content
	assert.Contains(t, prompt, dir)
	assert.Contains(t, prompt, "Prompt Engineer")
}

func TestAgent2_HandleResponse_ContinueOnReadBlocks(t *testing.T) {
	a := &Agent2{
		readFiles: make(map[string]string),
	}

	response := "I need to read:\n```read\npath/to/file.go\n```"
	action := a.HandleResponse(response)
	assert.Equal(t, ActionContinue, action.Type)
}

func TestAgent2_HandleResponse_CompleteNoReadBlocks(t *testing.T) {
	a := &Agent2{
		readFiles: make(map[string]string),
	}

	response := "Here is the enriched prompt:\n## Goal\nDo something."
	action := a.HandleResponse(response)
	assert.Equal(t, ActionComplete, action.Type)
}

func TestAgent2_HandleResponse_ContinuesWithReadBlocks(t *testing.T) {
	a := &Agent2{
		turnCount: 20,
		readFiles: make(map[string]string),
	}

	// Still continues if read blocks are present, regardless of turn count
	response := "```read\npath/to/file.go\n```"
	action := a.HandleResponse(response)
	assert.Equal(t, ActionContinue, action.Type)
}

func TestAgent2_HandleResponse_IncrementsTurnCount(t *testing.T) {
	a := &Agent2{
		readFiles: make(map[string]string),
	}

	a.HandleResponse("some response")
	assert.Equal(t, 1, a.turnCount)
	a.HandleResponse("another response")
	assert.Equal(t, 2, a.turnCount)
}

func TestAgent2_ReadFiles(t *testing.T) {
	a := &Agent2{
		readFiles: map[string]string{
			"a.go": "content a",
			"b.go": "content b",
		},
	}
	files := a.ReadFiles()
	assert.Len(t, files, 2)
	assert.Equal(t, "content a", files["a.go"])
}

func TestAgent2_AddReadFile(t *testing.T) {
	a := &Agent2{
		readFiles: make(map[string]string),
	}

	a.AddReadFile("path/to/file.go", "package main")
	assert.Equal(t, "package main", a.readFiles["path/to/file.go"])
	assert.Equal(t, EstimateTokens("package main"), a.usedTokens)
}

func TestAgent2_TokenBudgetRemaining(t *testing.T) {
	a := &Agent2{
		tokenBudget: 1000,
		usedTokens:  300,
		readFiles:   make(map[string]string),
	}
	assert.Equal(t, 700, a.TokenBudgetRemaining())
}

func TestAgent2_TokenBudgetRemaining_ClampsToZero(t *testing.T) {
	a := &Agent2{
		tokenBudget: 100,
		usedTokens:  500,
		readFiles:   make(map[string]string),
	}
	assert.Equal(t, 0, a.TokenBudgetRemaining())
}

func TestAgent2_TaskDir(t *testing.T) {
	a := &Agent2{taskDir: "/some/path"}
	assert.Equal(t, "/some/path", a.TaskDir())
}
