package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanFileTree(t *testing.T) {
	dir := t.TempDir()

	// Create structure: dir/src/main.go, dir/src/util/helper.go, dir/README.md
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "util"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("pkg"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "util", "helper.go"), []byte("pkg"), 0o644))

	result, err := ScanFileTree(dir, 3, 0, nil)
	require.NoError(t, err)

	assert.Contains(t, result, "src/")
	assert.Contains(t, result, "main.go")
	assert.Contains(t, result, "util/")
	assert.Contains(t, result, "helper.go")
	assert.Contains(t, result, "README.md")
}

func TestScanFileTree_Excludes(t *testing.T) {
	dir := t.TempDir()

	excludeDirs := []string{".git", "node_modules", "__pycache__", ".venv", "vendor", "build", "dist", ".fakeoid"}
	for _, d := range excludeDirs {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, d), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, d, "file.txt"), []byte("x"), 0o644))
	}
	// Add a non-excluded file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644))

	result, err := ScanFileTree(dir, 3, 0, nil)
	require.NoError(t, err)

	assert.Contains(t, result, "keep.txt")
	for _, d := range excludeDirs {
		assert.NotContains(t, result, d+"/", "should exclude %s", d)
	}
}

func TestScanFileTree_MaxDepth(t *testing.T) {
	dir := t.TempDir()

	// Create: dir/a/b/c/deep.txt (depth 3 from root)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b", "c"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "b", "c", "deep.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "shallow.txt"), []byte("x"), 0o644))

	result, err := ScanFileTree(dir, 2, 0, nil)
	require.NoError(t, err)

	assert.Contains(t, result, "shallow.txt")
	assert.NotContains(t, result, "deep.txt")
}

func TestScanFileTree_MaxLines(t *testing.T) {
	dir := t.TempDir()

	// Create 250 files to exceed 200-line cap
	for i := 0; i < 250; i++ {
		name := filepath.Join(dir, fmt.Sprintf("file-%04d.txt", i))
		_ = os.WriteFile(name, []byte("x"), 0o644)
	}

	result, err := ScanFileTree(dir, 3, 0, nil)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	// Should be capped: 200 entries + 1 truncation message
	assert.LessOrEqual(t, len(lines), 201)
	assert.Contains(t, result, "... (")
	assert.Contains(t, result, "more entries)")
}
