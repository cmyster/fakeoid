package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTestResultsFile_BasicNaming(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks")

	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteTestResultsFile(sb, dir, "001-slug-task.md", 1, true, 3, []string{"a_test.go", "b_test.go", "c_test.go"})
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "001-slug-task-tests-iter1.md"), path)
	assert.FileExists(t, path)
}

func TestWriteTestResultsFile_Content(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks")

	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteTestResultsFile(sb, dir, "001-slug-task.md", 1, true, 5, []string{"foo_test.go", "bar_test.go"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)

	assert.True(t, strings.Contains(content, "# Test Results"), "should contain heading")
	assert.True(t, strings.Contains(content, "**Result:** pass"), "should contain pass result")
	assert.True(t, strings.Contains(content, "**Tests run:** 5"), "should contain test count")
	assert.True(t, strings.Contains(content, "- foo_test.go"), "should list test files")
	assert.True(t, strings.Contains(content, "- bar_test.go"), "should list test files")
}

func TestWriteTestResultsFile_FailResult(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks")

	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteTestResultsFile(sb, dir, "001-slug-task.md", 1, false, 2, []string{"x_test.go"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), "**Result:** fail"))
}

func TestWriteTestResultsFile_Iteration3(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks")

	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteTestResultsFile(sb, dir, "001-slug-task.md", 3, true, 1, []string{"a_test.go"})
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "001-slug-task-tests-iter3.md"), path)
}

func TestWriteTestResultsFile_CreatesDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "deep", "nested", "tasks")

	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteTestResultsFile(sb, dir, "001-slug-task.md", 1, true, 0, nil)
	require.NoError(t, err)

	assert.FileExists(t, path)

	// Check that empty test files list produces "No test files written"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), "No test files written"))
}
