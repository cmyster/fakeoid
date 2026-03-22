package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent3_ImplementsAgent(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "001-test.md")
	require.NoError(t, os.WriteFile(taskFile, []byte("# Task: test"), 0o644))

	a := NewAgent3(dir, dir, taskFile, "# AST data")
	var _ Agent = a // compile-time interface check
	assert.NotNil(t, a)
}

func TestAgent3_Number(t *testing.T) {
	a := &Agent3{}
	assert.Equal(t, 3, a.Number())
}

func TestAgent3_Name(t *testing.T) {
	a := &Agent3{}
	assert.Equal(t, "Software Architect", a.Name())
}

func TestAgent3_SystemPrompt_ContainsContext(t *testing.T) {
	dir := t.TempDir()
	taskFile := filepath.Join(dir, "001-add-retry.md")
	taskContent := "# Task: Add retry logic\n\n## Description\nAdd retry to HTTP client."
	require.NoError(t, os.WriteFile(taskFile, []byte(taskContent), 0o644))

	astMarkdown := "## Package: agent\n\nfunc NewAgent4(cwd, taskDir, taskFile string) *Agent4"

	a := NewAgent3(dir, dir, taskFile, astMarkdown)
	prompt := a.SystemPrompt()

	assert.Contains(t, prompt, dir)
	assert.Contains(t, prompt, "Add retry logic")
	assert.Contains(t, prompt, astMarkdown)
	assert.Contains(t, prompt, "Software Architect")
}

func TestAgent3_SystemPrompt_EmptyTaskFile(t *testing.T) {
	dir := t.TempDir()

	a := NewAgent3(dir, dir, "", "# AST data")
	prompt := a.SystemPrompt()

	// Should not panic or error, just produce prompt without task content
	assert.Contains(t, prompt, dir)
	assert.Contains(t, prompt, "# AST data")
	assert.Contains(t, prompt, "Software Architect")
}

func TestAgent3_HandleResponse(t *testing.T) {
	a := &Agent3{}
	action := a.HandleResponse("Here is my change plan...")
	assert.Equal(t, ActionComplete, action.Type)
}

func TestAgent3_HandleResponse_AlwaysComplete(t *testing.T) {
	a := &Agent3{}

	// Multiple calls should always return ActionComplete
	for i := 0; i < 5; i++ {
		action := a.HandleResponse("response")
		assert.Equal(t, ActionComplete, action.Type)
	}
}

func TestWriteChangePlanFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir)
	require.NoError(t, err)
	defer sb.Close()

	content := "# Change Plan: Add retry\n\n## Rationale\nNeed retry logic."

	path, err := WriteChangePlanFile(sb, taskDir, "001-add-retry.md", content)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(taskDir, "001-add-retry-change-plan.md"), path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestWriteChangePlanFile_FilenameDeriving(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteChangePlanFile(sb, taskDir, "042-fix-bug.md", "plan content")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(taskDir, "042-fix-bug-change-plan.md"), path)
}

func TestWriteChangePlanFile_WritesThroughSandbox(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir)
	require.NoError(t, err)
	defer sb.Close()

	content := "test content"
	path, err := WriteChangePlanFile(sb, taskDir, "001-test.md", content)
	require.NoError(t, err)

	// Verify file exists on disk (written through sandbox)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}
