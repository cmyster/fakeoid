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

// TestAllEffectiveDefaults verifies every Effective* method returns the correct
// default when called on an empty (zero-value) ModelConfig.
func TestAllEffectiveDefaults(t *testing.T) {
	cfg := &ModelConfig{}

	// Existing fields
	assert.Equal(t, 8080, cfg.EffectivePort())
	assert.Equal(t, 16384, cfg.EffectiveCtxSize())

	// Model identity
	assert.Equal(t, DefaultModelRepo, cfg.EffectiveModelRepo())
	assert.Equal(t, DefaultModelFile, cfg.EffectiveModelFile())
	assert.Equal(t, DefaultModelURL, cfg.EffectiveModelURL())
	assert.Equal(t, DefaultModelHash, cfg.EffectiveModelHash())
	assert.Equal(t, DefaultModelSize, cfg.EffectiveModelSize())
	assert.Equal(t, DefaultModelName, cfg.EffectiveModelName())
	assert.Equal(t, DefaultModelPath(), cfg.EffectiveModelPath())

	// Server
	assert.Equal(t, "999", cfg.EffectiveGPULayers())
	assert.Equal(t, "on", cfg.EffectiveFlashAttn())
	assert.Equal(t, "127.0.0.1", cfg.EffectiveHost())
	assert.Equal(t, 200, cfg.EffectiveLogBufferMax())

	// Timeouts
	assert.Equal(t, 5, cfg.EffectiveKillTimeoutSec())
	assert.Equal(t, 500, cfg.EffectiveHealthPollMs())
	assert.Equal(t, 2000, cfg.EffectiveHealthTimeoutMs())
	assert.Equal(t, 10, cfg.EffectiveChatTimeoutSec())
	assert.Equal(t, 120, cfg.EffectiveStartupTimeoutSec())
	assert.Equal(t, 100, cfg.EffectiveSpinnerIntervalMs())

	// Token budget
	assert.Equal(t, 60, cfg.EffectiveTokenBudgetPct())
	assert.Equal(t, 80, cfg.EffectiveHistoryTrimPct())
	assert.Equal(t, 4, cfg.EffectiveTokenCharDivisor())

	// UI/Display
	assert.Equal(t, 500, cfg.EffectiveHistoryLimit())
	assert.Equal(t, 80, cfg.EffectiveTerminalWidthFallback())
	assert.Equal(t, 5, cfg.EffectiveStartupHistoryMax())
	assert.Equal(t, 40, cfg.EffectiveTaskNameTruncLen())
	assert.Equal(t, 50, cfg.EffectiveSlugMaxLen())

	// Filesystem
	assert.Equal(t, ".fakeoid", cfg.EffectiveConfigDirName())
	assert.Equal(t, "models", cfg.EffectiveModelSubdir())
	assert.Equal(t, "tasks", cfg.EffectiveTaskSubdir())
	assert.Equal(t, "history.json", cfg.EffectiveHistoryFile())
	assert.Equal(t, "config.json", cfg.EffectiveConfigFile())

	// File tree scanner
	assert.Equal(t, 3, cfg.EffectiveTreeMaxDepth())
	assert.Equal(t, 200, cfg.EffectiveTreeMaxLines())
	assert.Equal(t, []string{".git", "node_modules", "__pycache__", ".venv", "vendor", "build", "dist", ".fakeoid"}, cfg.EffectiveTreeExcludes())

	// Sandbox
	assert.Equal(t, []string{"/etc", "/proc"}, cfg.EffectiveReadAllowPaths())
	assert.Equal(t, []string{"cargo", "rustc", "make", "cmake", "gcc", "g++", "clang", "clang++", "npm", "npx", "node", "go", "python", "python3", "pip"}, cfg.EffectiveAllowedBuildCommands())

	// Streaming
	assert.Equal(t, 1024*1024, cfg.EffectiveSSEBufferSize())

	// Feedback loop
	assert.Equal(t, 10, cfg.EffectiveMaxIterations())
}

// TestLoadConfigAllFields writes a JSON with all fields set to non-default values,
// loads it, and verifies every field was deserialized correctly.
func TestLoadConfigAllFields(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := ModelConfig{
		ModelPath:             "/custom/model.gguf",
		Port:                  9090,
		CtxSize:               32768,
		ModelRepo:             "custom/repo",
		ModelFile:             "custom.gguf",
		ModelURL:              "https://example.com/model.gguf",
		ModelHash:             "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
		ModelSize:             10_000_000_000,
		ModelName:             "CustomModel",
		GPULayers:             "10",
		FlashAttn:             "off",
		Host:                  "0.0.0.0",
		LogBufferMax:          500,
		KillTimeoutSec:        10,
		HealthPollMs:          1000,
		HealthTimeoutMs:       5000,
		ChatTimeoutSec:        30,
		StartupTimeoutSec:     300,
		SpinnerIntervalMs:     200,
		TokenBudgetPct:        70,
		HistoryTrimPct:        90,
		TokenCharDivisor:      3,
		HistoryLimit:          1000,
		TerminalWidthFallback: 120,
		StartupHistoryMax:     10,
		TaskNameTruncLen:      60,
		SlugMaxLen:            30,
		ConfigDirName:         ".myapp",
		ModelSubdir:           "cache",
		TaskSubdir:            "jobs",
		HistoryFile:           "hist.json",
		ConfigFile:            "settings.json",
		TreeMaxDepth:          5,
		TreeMaxLines:          500,
		TreeExcludes:          []string{".svn", "target"},
		ReadAllowPaths:        []string{"/usr"},
		AllowedBuildCommands:  []string{"cargo", "make"},
		SSEBufferSize:         2 * 1024 * 1024,
		MaxIterations:         20,
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0644))

	loaded, err := LoadConfigFrom(configPath)
	require.NoError(t, err)

	assert.Equal(t, cfg.ModelPath, loaded.ModelPath)
	assert.Equal(t, cfg.Port, loaded.Port)
	assert.Equal(t, cfg.CtxSize, loaded.CtxSize)
	assert.Equal(t, cfg.ModelRepo, loaded.ModelRepo)
	assert.Equal(t, cfg.ModelFile, loaded.ModelFile)
	assert.Equal(t, cfg.ModelURL, loaded.ModelURL)
	assert.Equal(t, cfg.ModelHash, loaded.ModelHash)
	assert.Equal(t, cfg.ModelSize, loaded.ModelSize)
	assert.Equal(t, cfg.ModelName, loaded.ModelName)
	assert.Equal(t, cfg.GPULayers, loaded.GPULayers)
	assert.Equal(t, cfg.FlashAttn, loaded.FlashAttn)
	assert.Equal(t, cfg.Host, loaded.Host)
	assert.Equal(t, cfg.LogBufferMax, loaded.LogBufferMax)
	assert.Equal(t, cfg.KillTimeoutSec, loaded.KillTimeoutSec)
	assert.Equal(t, cfg.HealthPollMs, loaded.HealthPollMs)
	assert.Equal(t, cfg.HealthTimeoutMs, loaded.HealthTimeoutMs)
	assert.Equal(t, cfg.ChatTimeoutSec, loaded.ChatTimeoutSec)
	assert.Equal(t, cfg.StartupTimeoutSec, loaded.StartupTimeoutSec)
	assert.Equal(t, cfg.SpinnerIntervalMs, loaded.SpinnerIntervalMs)
	assert.Equal(t, cfg.TokenBudgetPct, loaded.TokenBudgetPct)
	assert.Equal(t, cfg.HistoryTrimPct, loaded.HistoryTrimPct)
	assert.Equal(t, cfg.TokenCharDivisor, loaded.TokenCharDivisor)
	assert.Equal(t, cfg.HistoryLimit, loaded.HistoryLimit)
	assert.Equal(t, cfg.TerminalWidthFallback, loaded.TerminalWidthFallback)
	assert.Equal(t, cfg.StartupHistoryMax, loaded.StartupHistoryMax)
	assert.Equal(t, cfg.TaskNameTruncLen, loaded.TaskNameTruncLen)
	assert.Equal(t, cfg.SlugMaxLen, loaded.SlugMaxLen)
	assert.Equal(t, cfg.ConfigDirName, loaded.ConfigDirName)
	assert.Equal(t, cfg.ModelSubdir, loaded.ModelSubdir)
	assert.Equal(t, cfg.TaskSubdir, loaded.TaskSubdir)
	assert.Equal(t, cfg.HistoryFile, loaded.HistoryFile)
	assert.Equal(t, cfg.ConfigFile, loaded.ConfigFile)
	assert.Equal(t, cfg.TreeMaxDepth, loaded.TreeMaxDepth)
	assert.Equal(t, cfg.TreeMaxLines, loaded.TreeMaxLines)
	assert.Equal(t, cfg.TreeExcludes, loaded.TreeExcludes)
	assert.Equal(t, cfg.ReadAllowPaths, loaded.ReadAllowPaths)
	assert.Equal(t, cfg.AllowedBuildCommands, loaded.AllowedBuildCommands)
	assert.Equal(t, cfg.SSEBufferSize, loaded.SSEBufferSize)
	assert.Equal(t, cfg.MaxIterations, loaded.MaxIterations)

	// Verify Effective* methods return the custom values (not defaults)
	assert.Equal(t, 9090, loaded.EffectivePort())
	assert.Equal(t, 32768, loaded.EffectiveCtxSize())
	assert.Equal(t, "custom/repo", loaded.EffectiveModelRepo())
	assert.Equal(t, "10", loaded.EffectiveGPULayers())
	assert.Equal(t, "off", loaded.EffectiveFlashAttn())
	assert.Equal(t, "0.0.0.0", loaded.EffectiveHost())
	assert.Equal(t, 500, loaded.EffectiveLogBufferMax())
	assert.Equal(t, 10, loaded.EffectiveKillTimeoutSec())
	assert.Equal(t, 30, loaded.EffectiveChatTimeoutSec())
	assert.Equal(t, 70, loaded.EffectiveTokenBudgetPct())
	assert.Equal(t, 90, loaded.EffectiveHistoryTrimPct())
	assert.Equal(t, 3, loaded.EffectiveTokenCharDivisor())
	assert.Equal(t, 1000, loaded.EffectiveHistoryLimit())
	assert.Equal(t, []string{".svn", "target"}, loaded.EffectiveTreeExcludes())
	assert.Equal(t, []string{"/usr"}, loaded.EffectiveReadAllowPaths())
	assert.Equal(t, []string{"cargo", "make"}, loaded.EffectiveAllowedBuildCommands())
	assert.Equal(t, 2*1024*1024, loaded.EffectiveSSEBufferSize())
	assert.Equal(t, 20, loaded.EffectiveMaxIterations())
}

// TestLoadConfigBackwardCompatibleExtended verifies that a 3-field config from
// the old format still works with the expanded struct.
func TestLoadConfigBackwardCompatibleExtended(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Old-format: only 3 fields
	data := []byte(`{"model_path":"/old/model.gguf","port":9090,"ctx_size":8192}`)
	require.NoError(t, os.WriteFile(configPath, data, 0644))

	cfg, err := LoadConfigFrom(configPath)
	require.NoError(t, err)

	// Explicit fields loaded correctly
	assert.Equal(t, "/old/model.gguf", cfg.ModelPath)
	assert.Equal(t, 9090, cfg.EffectivePort())
	assert.Equal(t, 8192, cfg.EffectiveCtxSize())

	// All new fields use defaults
	assert.Equal(t, DefaultModelRepo, cfg.EffectiveModelRepo())
	assert.Equal(t, "999", cfg.EffectiveGPULayers())
	assert.Equal(t, 200, cfg.EffectiveLogBufferMax())
	assert.Equal(t, 5, cfg.EffectiveKillTimeoutSec())
	assert.Equal(t, 60, cfg.EffectiveTokenBudgetPct())
	assert.Equal(t, 500, cfg.EffectiveHistoryLimit())
	assert.Equal(t, ".fakeoid", cfg.EffectiveConfigDirName())
	assert.Equal(t, 3, cfg.EffectiveTreeMaxDepth())
	assert.Equal(t, 10, cfg.EffectiveMaxIterations())
}

// TestEffectiveSliceFieldsNilVsEmpty verifies nil returns defaults, empty returns empty.
func TestEffectiveSliceFieldsNilVsEmpty(t *testing.T) {
	// Nil slices should return defaults
	cfg := &ModelConfig{}
	assert.NotEmpty(t, cfg.EffectiveTreeExcludes(), "nil TreeExcludes should return defaults")
	assert.NotEmpty(t, cfg.EffectiveReadAllowPaths(), "nil ReadAllowPaths should return defaults")
	assert.NotEmpty(t, cfg.EffectiveAllowedBuildCommands(), "nil AllowedBuildCommands should return defaults")

	// Empty (non-nil) slices should return empty -- user explicitly set to empty
	cfg2 := &ModelConfig{
		TreeExcludes:         []string{},
		ReadAllowPaths:       []string{},
		AllowedBuildCommands: []string{},
	}
	assert.Empty(t, cfg2.EffectiveTreeExcludes(), "empty TreeExcludes should return empty")
	assert.Empty(t, cfg2.EffectiveReadAllowPaths(), "empty ReadAllowPaths should return empty")
	assert.Empty(t, cfg2.EffectiveAllowedBuildCommands(), "empty AllowedBuildCommands should return empty")
}
