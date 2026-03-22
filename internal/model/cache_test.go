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

	name, size, exists, err := CachedModelInfoAt(modelPath, "")
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, int64(len(content)), size)
	assert.NotEmpty(t, name)
}

func TestCachedModelInfoMissing(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "nonexistent.gguf")

	_, _, exists, err := CachedModelInfoAt(modelPath, "")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestCachedModelInfoCustomPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create custom model file
	customPath := filepath.Join(tmpDir, "custom-model.gguf")
	content := []byte("custom model data")
	require.NoError(t, os.WriteFile(customPath, content, 0644))

	// Create config with model_path override
	configDir := filepath.Join(tmpDir, "config")
	require.NoError(t, os.MkdirAll(configDir, 0755))
	configPath := filepath.Join(configDir, "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"model_path":"`+customPath+`"}`), 0644))

	// Default path does not exist
	defaultPath := filepath.Join(tmpDir, "default.gguf")

	name, size, exists, err := CachedModelInfoAt(defaultPath, configPath)
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, int64(len(content)), size)
	assert.Contains(t, name, "custom-model.gguf")
}
