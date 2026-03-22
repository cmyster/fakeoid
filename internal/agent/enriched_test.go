package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseReadBlocks_SinglePath(t *testing.T) {
	input := "some text\n```read\npath/to/file.go\n```\nmore text"
	paths := ParseReadBlocks(input)
	assert.Equal(t, []string{"path/to/file.go"}, paths)
}

func TestParseReadBlocks_MultiplePaths(t *testing.T) {
	input := "```read\nfile1.go\nfile2.go\n```"
	paths := ParseReadBlocks(input)
	assert.Equal(t, []string{"file1.go", "file2.go"}, paths)
}

func TestParseReadBlocks_LanguagePathNotMatched(t *testing.T) {
	input := "```go:path/to/file.go\npackage main\n```"
	paths := ParseReadBlocks(input)
	assert.Nil(t, paths)
}

func TestParseReadBlocks_NoBlocks(t *testing.T) {
	input := "no blocks here"
	paths := ParseReadBlocks(input)
	assert.Nil(t, paths)
}

func TestParseReadBlocks_TrimsPaths(t *testing.T) {
	input := "```read\n  path/with/spaces.go  \n```"
	paths := ParseReadBlocks(input)
	assert.Equal(t, []string{"path/with/spaces.go"}, paths)
}

func TestParseReadBlocks_EmptyLinesIgnored(t *testing.T) {
	input := "```read\nfile1.go\n\nfile2.go\n```"
	paths := ParseReadBlocks(input)
	assert.Equal(t, []string{"file1.go", "file2.go"}, paths)
}

func TestParseReadBlocks_MultipleBlocks(t *testing.T) {
	input := "text\n```read\na.go\n```\nmiddle\n```read\nb.go\n```"
	paths := ParseReadBlocks(input)
	assert.Equal(t, []string{"a.go", "b.go"}, paths)
}

func TestEstimateTokens_Normal(t *testing.T) {
	assert.Equal(t, 1, EstimateTokens("abcd"))
}

func TestEstimateTokens_Empty(t *testing.T) {
	assert.Equal(t, 0, EstimateTokens(""))
}

func TestEstimateTokens_Eight(t *testing.T) {
	assert.Equal(t, 2, EstimateTokens("12345678"))
}

func TestTokenBudget_Normal(t *testing.T) {
	// ctxSize=8192, systemPromptChars=2000, taskChars=1000
	// budget = 8192*60/100 - 2000/4 - 1000/4 = 4915 - 500 - 250 = 4165
	result := TokenBudget(8192, 2000, 1000)
	assert.Equal(t, 4165, result)
}

func TestTokenBudget_ClampsToZero(t *testing.T) {
	// Small ctxSize where overhead exceeds budget
	result := TokenBudget(10, 10000, 10000)
	assert.Equal(t, 0, result)
}

func TestApplyTokenBudget_WithinBudget(t *testing.T) {
	files := []FileContent{
		{Path: "a.go", Content: "abcd"},     // 1 token
		{Path: "b.go", Content: "12345678"}, // 2 tokens
	}
	kept, omitted := ApplyTokenBudget(files, 10)
	assert.Len(t, kept, 2)
	assert.Empty(t, omitted)
}

func TestApplyTokenBudget_OverBudget(t *testing.T) {
	files := []FileContent{
		{Path: "a.go", Content: "abcd"},         // 1 token
		{Path: "b.go", Content: "12345678"},     // 2 tokens
		{Path: "c.go", Content: "1234567890ab"}, // 3 tokens
	}
	kept, omitted := ApplyTokenBudget(files, 3)
	assert.Len(t, kept, 2)
	assert.Equal(t, []string{"c.go"}, omitted)
}

func TestApplyTokenBudget_ZeroBudget(t *testing.T) {
	files := []FileContent{
		{Path: "a.go", Content: "abcd"},
	}
	kept, omitted := ApplyTokenBudget(files, 0)
	assert.Empty(t, kept)
	assert.Equal(t, []string{"a.go"}, omitted)
}

func TestApplyTokenBudget_DropsLastFirst(t *testing.T) {
	files := []FileContent{
		{Path: "a.go", Content: "abcd"},         // 1 token
		{Path: "b.go", Content: "abcdefgh"},     // 2 tokens
		{Path: "c.go", Content: "abcdefghijkl"}, // 3 tokens
	}
	// Budget of 3 allows a (1) + b (2) = 3, but c (3) would make 6 > 3
	kept, omitted := ApplyTokenBudget(files, 3)
	assert.Len(t, kept, 2)
	assert.Equal(t, "a.go", kept[0].Path)
	assert.Equal(t, "b.go", kept[1].Path)
	assert.Equal(t, []string{"c.go"}, omitted)
}

func TestWriteEnrichedFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir)
	require.NoError(t, err)
	defer sb.Close()

	content := "# Enriched prompt content"

	path, err := WriteEnrichedFile(sb, taskDir, "001-add-retry.md", content)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(taskDir, "001-add-retry-enriched.md"), path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestWriteEnrichedFile_FilenameDeriving(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir)
	require.NoError(t, err)
	defer sb.Close()

	path, err := WriteEnrichedFile(sb, taskDir, "042-fix-bug.md", "content")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(taskDir, "042-fix-bug-enriched.md"), path)
}

func TestWriteEnrichedFile_WritesThroughSandbox(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir)
	require.NoError(t, err)
	defer sb.Close()

	content := "test enriched content"
	path, err := WriteEnrichedFile(sb, taskDir, "001-test.md", content)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}
