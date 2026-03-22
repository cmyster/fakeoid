package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTaskFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks")

	sb, err := sandbox.New(root)
	require.NoError(t, err)
	defer sb.Close()

	content := "# Add retry mechanism\n\nImplement retries for HTTP calls."
	path, err := WriteTaskFile(sb, dir, content)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "001-add-retry-mechanism-task.md"), path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestWriteTaskFile_Sequential(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tasks")

	sb, err := sandbox.New(root)
	require.NoError(t, err)
	defer sb.Close()

	_, err = WriteTaskFile(sb, dir, "# First task\n\nDetails.")
	require.NoError(t, err)

	path2, err := WriteTaskFile(sb, dir, "# Second task\n\nMore details.")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "002-second-task-task.md"), path2)
}

func TestWriteTaskFile_CreatesDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "deep", "nested", "tasks")

	sb, err := sandbox.New(root)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteTaskFile(sb, dir, "# Some task\n\nBody.")
	require.NoError(t, err)

	assert.FileExists(t, path)
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Add Retry Mechanism for HTTP", "add-retry-mechanism-for-http"},
		{"Hello, World! This is a TEST.", "hello-world-this-is-a-test"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := generateSlug(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGenerateSlug_Empty(t *testing.T) {
	assert.Equal(t, "task", generateSlug(""))
	assert.Equal(t, "task", generateSlug("   "))
}

func TestGenerateSlug_MarkdownHeading(t *testing.T) {
	assert.Equal(t, "fix-the-bug", generateSlug("# Fix the Bug"))
	assert.Equal(t, "another-heading", generateSlug("## Another Heading"))
}

func TestGenerateSlug_Truncation(t *testing.T) {
	long := "This is a very long task title that should definitely be truncated at fifty characters or fewer"
	slug := generateSlug(long)
	assert.LessOrEqual(t, len(slug), 50)
	// Should not end with a hyphen
	assert.NotEqual(t, '-', slug[len(slug)-1])
}
