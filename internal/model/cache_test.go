package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureCacheDir(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, ".fakeoid", "models")

	path, err := EnsureCacheDirAt(cacheDir)
	assert.NoError(t, err)
	assert.Equal(t, cacheDir, path)

	info, err := os.Stat(cacheDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureCacheDirExists(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, ".fakeoid", "models")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))

	path, err := EnsureCacheDirAt(cacheDir)
	assert.NoError(t, err)
	assert.Equal(t, cacheDir, path)
}

func TestCachedModelInfoExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake model file at the "default" location
	modelsDir := filepath.Join(tmpDir, "models")
	require.NoError(t, os.MkdirAll(modelsDir, 0755))
	modelPath := filepath.Join(modelsDir, "test-model.gguf")
	content := []byte("fake model content")
	require.NoError(t, os.WriteFile(modelPath, content, 0644))

	name, size, exists, err := CachedModelInfoAt(modelPath)
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, int64(len(content)), size)
	assert.NotEmpty(t, name)
}

func TestCachedModelInfoMissing(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "nonexistent.gguf")

	_, _, exists, err := CachedModelInfoAt(modelPath)
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestCachedModelInfoFromConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create custom model file
	customPath := filepath.Join(tmpDir, "custom-model.gguf")
	content := []byte("custom model data")
	require.NoError(t, os.WriteFile(customPath, content, 0644))

	cfg := &ModelConfig{ModelPath: customPath}

	name, size, exists, err := CachedModelInfo(cfg)
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, int64(len(content)), size)
	assert.Contains(t, name, "custom-model.gguf")
}

func TestCachedModelInfoEmptyConfig(t *testing.T) {
	cfg := &ModelConfig{}
	_, _, exists, err := CachedModelInfo(cfg)
	assert.NoError(t, err)
	assert.False(t, exists)
}
