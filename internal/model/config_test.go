package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectiveModelPathFromFile(t *testing.T) {
	cfg := &ModelConfig{ModelFile: "test-model.gguf"}
	p := cfg.EffectiveModelPath()

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(home, ".fakeoid", "models", "test-model.gguf"), p)
}

func TestEffectiveModelPathExplicit(t *testing.T) {
	cfg := &ModelConfig{ModelPath: "/custom/path.gguf", ModelFile: "ignored.gguf"}
	assert.Equal(t, "/custom/path.gguf", cfg.EffectiveModelPath())
}

func TestEffectiveModelPathDefault(t *testing.T) {
	cfg := &ModelConfig{}
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	// Empty config should derive path from the default model file
	assert.Equal(t, filepath.Join(home, ".fakeoid", "models", "google_gemma-4-31B-it-Q4_K_M.gguf"), cfg.EffectiveModelPath())
}

func TestValidateModelIdentity(t *testing.T) {
	// Empty config passes — defaults provide model_file and model_name
	cfg := &ModelConfig{}
	assert.NoError(t, cfg.ValidateModelIdentity())

	// Explicit values also pass
	cfg = &ModelConfig{ModelFile: "model.gguf", ModelName: "MyModel"}
	assert.NoError(t, cfg.ValidateModelIdentity())
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
	assert.Equal(t, 32768, cfg.EffectiveCtxSize())
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
	assert.Equal(t, 32768, cfg.EffectiveCtxSize())
}

// TestAllEffectiveDefaults verifies every Effective* method returns the correct
// default when called on an empty (zero-value) ModelConfig.
func TestAllEffectiveDefaults(t *testing.T) {
	cfg := &ModelConfig{}

	// Existing fields
	assert.Equal(t, 8080, cfg.EffectivePort())
	assert.Equal(t, 32768, cfg.EffectiveCtxSize())

	// Model identity (defaults to Gemma-4-31B)
	assert.Equal(t, "bartowski/google_gemma-4-31B-it-GGUF", cfg.EffectiveModelRepo())
	assert.Equal(t, "google_gemma-4-31B-it-Q4_K_M.gguf", cfg.EffectiveModelFile())
	assert.Equal(t, "https://huggingface.co/bartowski/google_gemma-4-31B-it-GGUF/resolve/main/google_gemma-4-31B-it-Q4_K_M.gguf", cfg.EffectiveModelURL())
	assert.Equal(t, "", cfg.EffectiveModelHash())
	assert.Equal(t, int64(19598483328), cfg.EffectiveModelSize())
	assert.Equal(t, "Gemma-4-31B-Q4_K_M", cfg.EffectiveModelName())
	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".fakeoid", "models", "google_gemma-4-31B-it-Q4_K_M.gguf"), cfg.EffectiveModelPath())

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
	assert.Equal(t, 100, cfg.EffectiveMaxIterations())

	// GPU compute throttling
	assert.Equal(t, 90, cfg.EffectiveGPUComputePct())

	// GPU VRAM allocation
	assert.Equal(t, 95, cfg.EffectiveGPUMaxAllocPct())
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
		GPUComputePct:         75,
		GPUMaxAllocPct:        80,
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
	assert.Equal(t, cfg.GPUComputePct, loaded.GPUComputePct)
	assert.Equal(t, cfg.GPUMaxAllocPct, loaded.GPUMaxAllocPct)

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
	assert.Equal(t, 75, loaded.EffectiveGPUComputePct())
	assert.Equal(t, 80, loaded.EffectiveGPUMaxAllocPct())
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

	// Model identity fields use defaults when not set in config
	assert.Equal(t, "bartowski/google_gemma-4-31B-it-GGUF", cfg.EffectiveModelRepo())
	assert.Equal(t, "999", cfg.EffectiveGPULayers())
	assert.Equal(t, 200, cfg.EffectiveLogBufferMax())
	assert.Equal(t, 5, cfg.EffectiveKillTimeoutSec())
	assert.Equal(t, 60, cfg.EffectiveTokenBudgetPct())
	assert.Equal(t, 500, cfg.EffectiveHistoryLimit())
	assert.Equal(t, ".fakeoid", cfg.EffectiveConfigDirName())
	assert.Equal(t, 3, cfg.EffectiveTreeMaxDepth())
	assert.Equal(t, 100, cfg.EffectiveMaxIterations())
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

// TestConfigSampleIsValidAndComplete reads the actual config.json.sample file,
// strips // comment lines, unmarshals it, and verifies all fields match defaults.
func TestConfigSampleIsValidAndComplete(t *testing.T) {
	// Read the sample file from repo root (two dirs up from model/)
	samplePath := filepath.Join("..", "..", "config.json.sample")
	data, err := os.ReadFile(samplePath)
	require.NoError(t, err, "config.json.sample should exist at repo root")

	// Strip // comment lines
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "//") {
			lines = append(lines, line)
		}
	}
	cleaned := strings.Join(lines, "\n")

	var cfg ModelConfig
	require.NoError(t, json.Unmarshal([]byte(cleaned), &cfg), "sample file should be valid JSON after stripping comments")

	// Verify sample has required model identity fields set
	assert.NotEmpty(t, cfg.ModelFile, "model_file must be set in sample")
	assert.True(t, strings.HasSuffix(cfg.ModelFile, ".gguf"), "model_file should end with .gguf")
	assert.NotEmpty(t, cfg.ModelName, "model_name must be set in sample")
	assert.NoError(t, cfg.ValidateModelIdentity(), "sample should pass model validation")

	// Verify sample non-model defaults match Effective* defaults from an empty config
	empty := &ModelConfig{}

	// Existing fields
	assert.Equal(t, empty.EffectivePort(), cfg.Port, "port")
	assert.Equal(t, empty.EffectiveCtxSize(), cfg.CtxSize, "ctx_size")

	// Server
	assert.Equal(t, empty.EffectiveGPULayers(), cfg.GPULayers, "gpu_layers")
	assert.Equal(t, empty.EffectiveFlashAttn(), cfg.FlashAttn, "flash_attn")
	assert.Equal(t, empty.EffectiveHost(), cfg.Host, "host")
	assert.Equal(t, empty.EffectiveLogBufferMax(), cfg.LogBufferMax, "log_buffer_max")

	// Timeouts
	assert.Equal(t, empty.EffectiveKillTimeoutSec(), cfg.KillTimeoutSec, "kill_timeout_seconds")
	assert.Equal(t, empty.EffectiveHealthPollMs(), cfg.HealthPollMs, "health_poll_ms")
	assert.Equal(t, empty.EffectiveHealthTimeoutMs(), cfg.HealthTimeoutMs, "health_timeout_ms")
	assert.Equal(t, empty.EffectiveChatTimeoutSec(), cfg.ChatTimeoutSec, "chat_timeout_seconds")
	assert.Equal(t, empty.EffectiveStartupTimeoutSec(), cfg.StartupTimeoutSec, "startup_timeout_seconds")
	assert.Equal(t, empty.EffectiveSpinnerIntervalMs(), cfg.SpinnerIntervalMs, "spinner_interval_ms")

	// Token budget
	assert.Equal(t, empty.EffectiveTokenBudgetPct(), cfg.TokenBudgetPct, "token_budget_pct")
	assert.Equal(t, empty.EffectiveHistoryTrimPct(), cfg.HistoryTrimPct, "history_trim_pct")
	assert.Equal(t, empty.EffectiveTokenCharDivisor(), cfg.TokenCharDivisor, "token_char_divisor")

	// UI/Display
	assert.Equal(t, empty.EffectiveHistoryLimit(), cfg.HistoryLimit, "history_limit")
	assert.Equal(t, empty.EffectiveTerminalWidthFallback(), cfg.TerminalWidthFallback, "terminal_width_fallback")
	assert.Equal(t, empty.EffectiveStartupHistoryMax(), cfg.StartupHistoryMax, "startup_history_max")
	assert.Equal(t, empty.EffectiveTaskNameTruncLen(), cfg.TaskNameTruncLen, "task_name_trunc_len")
	assert.Equal(t, empty.EffectiveSlugMaxLen(), cfg.SlugMaxLen, "slug_max_len")

	// Filesystem
	assert.Equal(t, empty.EffectiveConfigDirName(), cfg.ConfigDirName, "config_dir_name")
	assert.Equal(t, empty.EffectiveModelSubdir(), cfg.ModelSubdir, "model_subdir")
	assert.Equal(t, empty.EffectiveTaskSubdir(), cfg.TaskSubdir, "task_subdir")
	assert.Equal(t, empty.EffectiveHistoryFile(), cfg.HistoryFile, "history_file")
	assert.Equal(t, empty.EffectiveConfigFile(), cfg.ConfigFile, "config_file")

	// File tree scanner
	assert.Equal(t, empty.EffectiveTreeMaxDepth(), cfg.TreeMaxDepth, "tree_max_depth")
	assert.Equal(t, empty.EffectiveTreeMaxLines(), cfg.TreeMaxLines, "tree_max_lines")
	assert.Equal(t, empty.EffectiveTreeExcludes(), cfg.TreeExcludes, "tree_excludes")

	// Sandbox
	assert.Equal(t, empty.EffectiveReadAllowPaths(), cfg.ReadAllowPaths, "read_allow_paths")
	assert.Equal(t, empty.EffectiveAllowedBuildCommands(), cfg.AllowedBuildCommands, "allowed_build_commands")

	// Streaming
	assert.Equal(t, empty.EffectiveSSEBufferSize(), cfg.SSEBufferSize, "sse_buffer_size")

	// Feedback loop
	assert.Equal(t, empty.EffectiveMaxIterations(), cfg.MaxIterations, "max_iterations")

	// GPU compute throttling
	assert.Equal(t, empty.EffectiveGPUComputePct(), cfg.GPUComputePct, "gpu_compute_pct")

	// GPU VRAM allocation
	assert.Equal(t, empty.EffectiveGPUMaxAllocPct(), cfg.GPUMaxAllocPct, "gpu_max_alloc_pct")
}

func TestEffectiveGPUComputePctDefault(t *testing.T) {
	cfg := &ModelConfig{}
	assert.Equal(t, 90, cfg.EffectiveGPUComputePct())
}

func TestEffectiveGPUComputePctCustom(t *testing.T) {
	cfg := &ModelConfig{GPUComputePct: 75}
	assert.Equal(t, 75, cfg.EffectiveGPUComputePct())
}

func TestEffectiveGPUComputePctFloor(t *testing.T) {
	cfg := &ModelConfig{GPUComputePct: 5}
	assert.Equal(t, 10, cfg.EffectiveGPUComputePct())
}

func TestEffectiveGPUComputePctCap(t *testing.T) {
	cfg := &ModelConfig{GPUComputePct: 150}
	assert.Equal(t, 100, cfg.EffectiveGPUComputePct())
}

func TestEffectiveGPUMaxAllocPctDefault(t *testing.T) {
	cfg := &ModelConfig{}
	assert.Equal(t, 95, cfg.EffectiveGPUMaxAllocPct())
}

func TestEffectiveGPUMaxAllocPctCustom(t *testing.T) {
	cfg := &ModelConfig{GPUMaxAllocPct: 80}
	assert.Equal(t, 80, cfg.EffectiveGPUMaxAllocPct())
}

func TestEffectiveGPUMaxAllocPctFloor(t *testing.T) {
	cfg := &ModelConfig{GPUMaxAllocPct: 5}
	assert.Equal(t, 10, cfg.EffectiveGPUMaxAllocPct())
}

func TestEffectiveGPUMaxAllocPctCap(t *testing.T) {
	cfg := &ModelConfig{GPUMaxAllocPct: 150}
	assert.Equal(t, 100, cfg.EffectiveGPUMaxAllocPct())
}

func TestCalcAutoGPULayers(t *testing.T) {
	tests := []struct {
		name      string
		vramKB    uint64
		modelSize int64
		check     func(t *testing.T, result string)
	}{
		{
			name:      "24GB VRAM with int64(19_900_000_000) fits all layers (4GB headroom)",
			vramKB:    25149440,
			modelSize: int64(19_900_000_000),
			check: func(t *testing.T, result string) {
				assert.Equal(t, "999", result, "24GB VRAM should fit all layers with 4GB headroom (20GB usable > 19.9GB model)")
			},
		},
		{
			name:      "16GB VRAM with int64(19_900_000_000) does not fit all layers",
			vramKB:    16777216,
			modelSize: int64(19_900_000_000),
			check: func(t *testing.T, result string) {
				val, err := strconv.Atoi(result)
				require.NoError(t, err)
				assert.Less(t, val, 999, "16GB VRAM should not fit all layers")
				assert.Greater(t, val, 0, "should have at least 1 layer")
			},
		},
		{
			name:      "vramKB=0 returns fallback 999",
			vramKB:    0,
			modelSize: int64(19_900_000_000),
			check: func(t *testing.T, result string) {
				assert.Equal(t, "999", result)
			},
		},
		{
			name:      "modelSize=0 returns fallback 999",
			vramKB:    25149440,
			modelSize: 0,
			check: func(t *testing.T, result string) {
				assert.Equal(t, "999", result)
			},
		},
		{
			name:      "8GB VRAM with int64(19_900_000_000) returns small positive value",
			vramKB:    8388608,
			modelSize: int64(19_900_000_000),
			check: func(t *testing.T, result string) {
				val, err := strconv.Atoi(result)
				require.NoError(t, err)
				assert.GreaterOrEqual(t, val, 1, "should have at least 1 layer")
				assert.Less(t, val, 999, "8GB cannot fit all layers")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalcAutoGPULayers(tt.vramKB, tt.modelSize)
			tt.check(t, result)
		})
	}
}
