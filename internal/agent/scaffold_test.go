package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractFilePaths_BacktickWrapped(t *testing.T) {
	input := "modify `internal/agent/agent3.go`"
	paths := ExtractFilePaths(input)
	assert.Equal(t, []string{"internal/agent/agent3.go"}, paths)
}

func TestExtractFilePaths_MultipleBacktick(t *testing.T) {
	input := "create `cmd/main.go` and `internal/config/config.go`"
	paths := ExtractFilePaths(input)
	assert.Equal(t, []string{"cmd/main.go", "internal/config/config.go"}, paths)
}

func TestExtractFilePaths_NoFilePaths(t *testing.T) {
	input := "This is just text without file references"
	paths := ExtractFilePaths(input)
	assert.Empty(t, paths)
}

func TestExtractFilePaths_IgnoresNonFileLike(t *testing.T) {
	// No extension and no slash -- should be ignored
	input := "modify `something` and `another-thing`"
	paths := ExtractFilePaths(input)
	assert.Empty(t, paths)
}

func TestExtractFilePaths_BarePaths(t *testing.T) {
	input := "- internal/foo/bar.go\n- internal/baz/qux.go"
	paths := ExtractFilePaths(input)
	assert.Equal(t, []string{"internal/foo/bar.go", "internal/baz/qux.go"}, paths)
}

func TestExtractFilePaths_Deduplicated(t *testing.T) {
	input := "modify `internal/agent/agent3.go` and also `internal/agent/agent3.go`"
	paths := ExtractFilePaths(input)
	assert.Equal(t, []string{"internal/agent/agent3.go"}, paths)
}

func TestExtractFilePaths_MixedExtensions(t *testing.T) {
	input := "edit `config.yaml` and `main.py` and `app.ts`"
	paths := ExtractFilePaths(input)
	assert.Equal(t, []string{"config.yaml", "main.py", "app.ts"}, paths)
}

func TestScaffoldFiles_NewGoFile(t *testing.T) {
	root := t.TempDir()
	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	errs := ScaffoldFiles(sb, []string{"cmd/main.go"})
	assert.Empty(t, errs)

	data, err := os.ReadFile(filepath.Join(root, "cmd", "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package cmd\n", string(data))
}

func TestScaffoldFiles_NewNonGoFile(t *testing.T) {
	root := t.TempDir()
	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	errs := ScaffoldFiles(sb, []string{"config/app.yaml"})
	assert.Empty(t, errs)

	data, err := os.ReadFile(filepath.Join(root, "config", "app.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "", string(data))
}

func TestScaffoldFiles_ExistingFile(t *testing.T) {
	root := t.TempDir()
	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	// Create an existing file
	require.NoError(t, os.MkdirAll(filepath.Join(root, "internal"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "internal", "existing.go"), []byte("package internal\n"), 0o644))

	errs := ScaffoldFiles(sb, []string{"internal/existing.go"})
	assert.Empty(t, errs)

	data, err := os.ReadFile(filepath.Join(root, "internal", "existing.go"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "// TODO: Agent 3 flagged for modification")
	assert.Contains(t, string(data), "package internal")
}

func TestScaffoldFiles_NonFatal(t *testing.T) {
	root := t.TempDir()
	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	// Mix of valid and potentially problematic paths
	errs := ScaffoldFiles(sb, []string{"valid/file.go", "another/valid.go"})
	// Should not panic, may or may not have errors
	_ = errs
}

func TestScaffoldFiles_RootGoFile(t *testing.T) {
	root := t.TempDir()
	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	errs := ScaffoldFiles(sb, []string{"main.go"})
	assert.Empty(t, errs)

	data, err := os.ReadFile(filepath.Join(root, "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main\n", string(data))
}

func TestScaffoldFiles_MultipleFiles(t *testing.T) {
	root := t.TempDir()
	sb, err := sandbox.New(root, nil)
	require.NoError(t, err)
	defer sb.Close()

	paths := []string{"cmd/main.go", "internal/config/config.go", "README.md"}
	errs := ScaffoldFiles(sb, paths)
	assert.Empty(t, errs)

	// Verify all files exist
	assert.FileExists(t, filepath.Join(root, "cmd", "main.go"))
	assert.FileExists(t, filepath.Join(root, "internal", "config", "config.go"))
	assert.FileExists(t, filepath.Join(root, "README.md"))
}
