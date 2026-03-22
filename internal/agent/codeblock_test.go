package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCodeBlocks_SingleBlock(t *testing.T) {
	input := "```go:main.go\npackage main\n```"
	blocks := ParseCodeBlocks(input)
	require.Len(t, blocks, 1)
	assert.Equal(t, "main.go", blocks[0].FilePath)
	assert.Equal(t, "go", blocks[0].Language)
	assert.Equal(t, "package main\n", blocks[0].Content)
}

func TestParseCodeBlocks_MultipleBlocks(t *testing.T) {
	input := "Here is the code:\n```go:cmd/main.go\npackage main\n\nfunc main() {}\n```\nAnd the test:\n```go:cmd/main_test.go\npackage main\n\nimport \"testing\"\n```"
	blocks := ParseCodeBlocks(input)
	require.Len(t, blocks, 2)
	assert.Equal(t, "cmd/main.go", blocks[0].FilePath)
	assert.Equal(t, "cmd/main_test.go", blocks[1].FilePath)
	assert.Contains(t, blocks[0].Content, "func main()")
	assert.Contains(t, blocks[1].Content, "import \"testing\"")
}

func TestParseCodeBlocks_IgnoresUnannotated(t *testing.T) {
	input := "Example:\n```go\nfmt.Println(\"hello\")\n```\nNow the real file:\n```go:hello.go\npackage main\n```"
	blocks := ParseCodeBlocks(input)
	require.Len(t, blocks, 1)
	assert.Equal(t, "hello.go", blocks[0].FilePath)
}

func TestParseCodeBlocks_NestedFences(t *testing.T) {
	// 4-backtick fence wrapping a 3-backtick example
	input := "````go:example.go\nHere is code:\n```\nnested fence\n```\nEnd of file\n````"
	blocks := ParseCodeBlocks(input)
	require.Len(t, blocks, 1)
	assert.Equal(t, "example.go", blocks[0].FilePath)
	assert.Contains(t, blocks[0].Content, "```")
	assert.Contains(t, blocks[0].Content, "nested fence")
}

func TestParseCodeBlocks_NoBlocks(t *testing.T) {
	input := "This is just text with no code blocks at all."
	blocks := ParseCodeBlocks(input)
	assert.Empty(t, blocks)
}

func TestParseCodeBlocks_EmptyContent(t *testing.T) {
	input := "```go:empty.go\n```"
	blocks := ParseCodeBlocks(input)
	require.Len(t, blocks, 1)
	assert.Equal(t, "empty.go", blocks[0].FilePath)
	assert.Equal(t, "", blocks[0].Content)
}

func TestWriteCodeBlocks_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	blocks := []CodeBlock{
		{FilePath: "hello.go", Language: "go", Content: "package main\n"},
	}
	results, blocked := WriteCodeBlocks(sb, blocks)
	assert.Empty(t, blocked)
	require.Len(t, results, 1)
	assert.Equal(t, "hello.go", results[0].Path)
	assert.Equal(t, "created", results[0].Action)

	content, err := os.ReadFile(filepath.Join(dir, "hello.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main\n", string(content))
}

func TestWriteCodeBlocks_CreatesSubdirectories(t *testing.T) {
	dir := t.TempDir()
	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	blocks := []CodeBlock{
		{FilePath: "internal/pkg/util.go", Language: "go", Content: "package pkg\n"},
	}
	results, blocked := WriteCodeBlocks(sb, blocks)
	assert.Empty(t, blocked)
	require.Len(t, results, 1)
	assert.Equal(t, "created", results[0].Action)

	_, statErr := os.Stat(filepath.Join(dir, "internal", "pkg", "util.go"))
	assert.NoError(t, statErr)
}

func TestWriteCodeBlocks_ModifiedExisting(t *testing.T) {
	dir := t.TempDir()
	// Create existing file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.go"), []byte("old"), 0o644))

	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	blocks := []CodeBlock{
		{FilePath: "existing.go", Language: "go", Content: "new content\n"},
	}
	results, blocked := WriteCodeBlocks(sb, blocks)
	assert.Empty(t, blocked)
	require.Len(t, results, 1)
	assert.Equal(t, "modified", results[0].Action)
}

func TestWriteCodeBlocks_BlocksPathTraversal(t *testing.T) {
	dir := t.TempDir()
	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	blocks := []CodeBlock{
		{FilePath: "../escape.go", Language: "go", Content: "bad\n"},
	}
	results, blocked := WriteCodeBlocks(sb, blocks)
	assert.Empty(t, results)
	require.Len(t, blocked, 1)
	assert.Equal(t, "../escape.go", blocked[0].Path)
}

func TestWriteCodeBlocks_BlocksAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	blocks := []CodeBlock{
		{FilePath: "/etc/passwd", Language: "go", Content: "bad\n"},
	}
	results, blocked := WriteCodeBlocks(sb, blocks)
	assert.Empty(t, results)
	require.Len(t, blocked, 1)
	assert.Contains(t, blocked[0].Reason, "blocked")
}
