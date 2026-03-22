package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConstants(t *testing.T) {
	assert.Contains(t, DefaultModelURL, "bartowski/Qwen2.5-Coder-32B-Instruct-GGUF")
	assert.True(t, strings.HasSuffix(DefaultModelFile, ".gguf"))
	assert.Greater(t, DefaultModelSize, int64(0))
	assert.Len(t, DefaultModelHash, 64, "SHA256 hash should be 64 hex characters")

	// Verify hash is valid hex
	for _, c := range DefaultModelHash {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"Hash character '%c' is not valid hex", c)
	}
}

func TestDefaultModelPath(t *testing.T) {
	p := DefaultModelPath()

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(p, filepath.Join(home, ".fakeoid", "models")),
		"Path should be under ~/.fakeoid/models/")
	assert.True(t, strings.HasSuffix(p, DefaultModelFile),
		"Path should end with DefaultModelFile")
}

func TestLoadConfigNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg, err := LoadConfigFrom(configPath)
	assert.NoError(t, err)
	assert.Equal(t, "", cfg.ModelPath, "ModelPath should be empty when config file does not exist")
}

func TestLoadConfigWithOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	data, err := json.Marshal(ModelConfig{ModelPath: "/custom/path/model.gguf"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0644))

	cfg, err := LoadConfigFrom(configPath)
	assert.NoError(t, err)
	assert.Equal(t, "/custom/path/model.gguf", cfg.ModelPath)
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	require.NoError(t, os.WriteFile(configPath, []byte("{invalid json"), 0644))

	_, err := LoadConfigFrom(configPath)
	assert.Error(t, err)
}

func TestEffectivePortDefault(t *testing.T) {
	cfg := &ModelConfig{}
	assert.Equal(t, 8080, cfg.EffectivePort())
}

func TestEffectivePortCustom(t *testing.T) {
	cfg := &ModelConfig{Port: 9090}
	assert.Equal(t, 9090, cfg.EffectivePort())
}

func TestEffectiveCtxSizeDefault(t *testing.T) {
	cfg := &ModelConfig{}
	assert.Equal(t, 16384, cfg.EffectiveCtxSize())
}

func TestEffectiveCtxSizeCustom(t *testing.T) {
	cfg := &ModelConfig{CtxSize: 16384}
	assert.Equal(t, 16384, cfg.EffectiveCtxSize())
}

func TestLoadConfigWithPortAndCtxSize(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	data := []byte(`{"port": 9090, "ctx_size": 16384, "model_path": "/custom/model.gguf"}`)
	require.NoError(t, os.WriteFile(configPath, data, 0644))

	cfg, err := LoadConfigFrom(configPath)
	require.NoError(t, err)
	assert.Equal(t, "/custom/model.gguf", cfg.ModelPath)
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, 16384, cfg.CtxSize)
	assert.Equal(t, 9090, cfg.EffectivePort())
	assert.Equal(t, 16384, cfg.EffectiveCtxSize())
}

func TestLoadConfigBackwardCompatible(t *testing.T) {
	// Config with only model_path (old format) should still work
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	data, err := json.Marshal(ModelConfig{ModelPath: "/old/model.gguf"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0644))

	cfg, err := LoadConfigFrom(configPath)
	require.NoError(t, err)
	assert.Equal(t, "/old/model.gguf", cfg.ModelPath)
	assert.Equal(t, 8080, cfg.EffectivePort())
	assert.Equal(t, 16384, cfg.EffectiveCtxSize())
}
