package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteCorrectionFile_Iter1Naming(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks")

	sb, err := sandbox.New(root)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteCorrectionFile(sb, dir, "001-slug-task.md", 1, "correction content")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "001-slug-task-correction-iter1.md"), path)
	assert.FileExists(t, path)
}

func TestWriteCorrectionFile_Iter3Naming(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks")

	sb, err := sandbox.New(root)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteCorrectionFile(sb, dir, "001-slug-task.md", 3, "correction content")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "001-slug-task-correction-iter3.md"), path)
}

func TestWriteCorrectionFile_ContentMatches(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks")

	sb, err := sandbox.New(root)
	require.NoError(t, err)
	defer sb.Close()

	content := "## CORRECTION NEEDED\n### Plan Drift\nSome drift here\n### Missing Requirements\nNone\n### Suggested Fixes\nFix X"
	path, err := WriteCorrectionFile(sb, dir, "001-slug-task.md", 1, content)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestWriteCorrectionFile_ReturnsAbsolutePath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks")

	sb, err := sandbox.New(root)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteCorrectionFile(sb, dir, "001-slug-task.md", 1, "content")
	require.NoError(t, err)

	assert.True(t, filepath.IsAbs(path), "returned path should be absolute: %s", path)
}

func TestWriteCorrectionFile_NilSandbox(t *testing.T) {
	_, err := WriteCorrectionFile(nil, "/tmp/tasks", "001-slug-task.md", 1, "content")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox is nil")
}
