package model

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
)


// ModelConfig holds user configuration overrides loaded from config.json.
// Zero-value fields use hardcoded defaults via Effective* methods.
type ModelConfig struct {
	// Existing fields (backward compatible)
	ModelPath string `json:"model_path"`
	Port      int    `json:"port"`
	CtxSize   int    `json:"ctx_size"`

	// Model identity
	ModelRepo string `json:"model_repo"`
	ModelFile string `json:"model_file"`
	ModelURL  string `json:"model_url"`
	ModelHash string `json:"model_hash"`
	ModelSize int64  `json:"model_size"`
	ModelName string `json:"model_name"`

	// Server
	GPULayers     string `json:"gpu_layers"`
	FlashAttn     string `json:"flash_attn"`
	Host          string `json:"host"`
	LogBufferMax  int    `json:"log_buffer_max"`
	GPUComputePct  int    `json:"gpu_compute_pct"`
	GPUMaxAllocPct int    `json:"gpu_max_alloc_pct"`
	CacheTypeK     string `json:"cache_type_k"`
	CacheTypeV     string `json:"cache_type_v"`

	// Timeouts
	KillTimeoutSec    int `json:"kill_timeout_seconds"`
	HealthPollMs      int `json:"health_poll_ms"`
	HealthTimeoutMs   int `json:"health_timeout_ms"`
	ChatTimeoutSec    int `json:"chat_timeout_seconds"`
	StartupTimeoutSec int `json:"startup_timeout_seconds"`
	SpinnerIntervalMs int `json:"spinner_interval_ms"`

	// Token budget
	TokenBudgetPct   int `json:"token_budget_pct"`
	HistoryTrimPct   int `json:"history_trim_pct"`
	TokenCharDivisor int `json:"token_char_divisor"`

	// UI/Display
	HistoryLimit          int `json:"history_limit"`
	TerminalWidthFallback int `json:"terminal_width_fallback"`
	StartupHistoryMax     int `json:"startup_history_max"`
	TaskNameTruncLen      int `json:"task_name_trunc_len"`
	SlugMaxLen            int `json:"slug_max_len"`

	// Filesystem
	ConfigDirName string `json:"config_dir_name"`
	ModelSubdir   string `json:"model_subdir"`
	TaskSubdir    string `json:"task_subdir"`
	HistoryFile   string `json:"history_file"`
	ConfigFile    string `json:"config_file"`

	// File tree scanner
	TreeMaxDepth int      `json:"tree_max_depth"`
	TreeMaxLines int      `json:"tree_max_lines"`
	TreeExcludes []string `json:"tree_excludes"`

	// Sandbox
	ReadAllowPaths       []string `json:"read_allow_paths"`
	AllowedBuildCommands []string `json:"allowed_build_commands"`

	// Streaming
	SSEBufferSize int `json:"sse_buffer_size"`

	// Feedback loop
	MaxIterations int `json:"max_iterations"`
}

// --- Effective* methods: return configured value or hardcoded default ---

// EffectivePort returns the configured port, or 8080 if not set.
func (c *ModelConfig) EffectivePort() int {
	if c.Port == 0 {
		return 8080
	}
	return c.Port
}

// EffectiveCtxSize returns the configured context size, or 32768 if not set.
func (c *ModelConfig) EffectiveCtxSize() int {
	if c.CtxSize == 0 {
		return 32768
	}
	return c.CtxSize
}

// EffectiveModelRepo returns the configured model repo, or the default Gemma repo.
func (c *ModelConfig) EffectiveModelRepo() string {
	if c.ModelRepo == "" {
		return "bartowski/google_gemma-4-31B-it-GGUF"
	}
	return c.ModelRepo
}

// EffectiveModelFile returns the configured model file, or the default Gemma GGUF.
func (c *ModelConfig) EffectiveModelFile() string {
	if c.ModelFile == "" {
		return "google_gemma-4-31B-it-Q4_K_M.gguf"
	}
	return c.ModelFile
}

// EffectiveModelURL returns the configured model URL, or the default HuggingFace URL.
func (c *ModelConfig) EffectiveModelURL() string {
	if c.ModelURL == "" {
		return "https://huggingface.co/bartowski/google_gemma-4-31B-it-GGUF/resolve/main/google_gemma-4-31B-it-Q4_K_M.gguf"
	}
	return c.ModelURL
}

// EffectiveModelHash returns the configured model hash (empty means skip verification).
func (c *ModelConfig) EffectiveModelHash() string {
	return c.ModelHash
}

// EffectiveModelSize returns the configured model size in bytes, or the default (~19.6GB).
func (c *ModelConfig) EffectiveModelSize() int64 {
	if c.ModelSize == 0 {
		return 19598483328
	}
	return c.ModelSize
}

// EffectiveModelName returns the configured model name, or the default Gemma name.
func (c *ModelConfig) EffectiveModelName() string {
	if c.ModelName == "" {
		return "Gemma-4-31B-Q4_K_M"
	}
	return c.ModelName
}

// EffectiveModelPath returns the configured model path, or derives it from EffectiveModelFile.
func (c *ModelConfig) EffectiveModelPath() string {
	if c.ModelPath != "" {
		return c.ModelPath
	}
	if f := c.EffectiveModelFile(); f != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		return filepath.Join(home, ".fakeoid", "models", f)
	}
	return ""
}

// ValidateModelIdentity returns an error if required model fields are missing
// after applying defaults. This should always pass with built-in defaults.
func (c *ModelConfig) ValidateModelIdentity() error {
	if c.EffectiveModelFile() == "" {
		return errors.New("model_file is required in config.json (e.g. \"google_gemma-4-31B-it-Q4_K_M.gguf\")")
	}
	if c.EffectiveModelName() == "" {
		return errors.New("model_name is required in config.json (e.g. \"Gemma-4-31B-Q4_K_M\")")
	}
	return nil
}

// EffectiveGPULayers returns the configured GPU layers or "999".
// NOTE: root.go intentionally passes the raw GPULayers field to ServerConfig
// so that an empty value triggers CalcAutoGPULayers (VRAM-based auto-detection)
// in lifecycle.go, which is smarter than hardcoding "999". This default only
// applies when EffectiveGPULayers is called directly.
func (c *ModelConfig) EffectiveGPULayers() string {
	if c.GPULayers == "" {
		return "999"
	}
	return c.GPULayers
}

// EffectiveFlashAttn returns the configured flash attention setting or "on".
func (c *ModelConfig) EffectiveFlashAttn() string {
	if c.FlashAttn == "" {
		return "on"
	}
	return c.FlashAttn
}

// EffectiveHost returns the configured host or "127.0.0.1".
func (c *ModelConfig) EffectiveHost() string {
	if c.Host == "" {
		return "127.0.0.1"
	}
	return c.Host
}

// EffectiveLogBufferMax returns the configured log buffer max or 200.
func (c *ModelConfig) EffectiveLogBufferMax() int {
	if c.LogBufferMax == 0 {
		return 200
	}
	return c.LogBufferMax
}

// EffectiveGPUComputePct returns the configured GPU compute percentage or 90.
// Clamps to the range [10, 100].
func (c *ModelConfig) EffectiveGPUComputePct() int {
	if c.GPUComputePct == 0 {
		return 90
	}
	if c.GPUComputePct < 10 {
		return 10
	}
	if c.GPUComputePct > 100 {
		return 100
	}
	return c.GPUComputePct
}

// EffectiveGPUMaxAllocPct returns the configured GPU max allocation percentage or 95.
// Clamps to the range [10, 100].
func (c *ModelConfig) EffectiveGPUMaxAllocPct() int {
	if c.GPUMaxAllocPct == 0 {
		return 95
	}
	if c.GPUMaxAllocPct < 10 {
		return 10
	}
	if c.GPUMaxAllocPct > 100 {
		return 100
	}
	return c.GPUMaxAllocPct
}

// EffectiveCacheTypeK returns the KV cache quantization type for keys,
// or "q8_0" if not set. q8_0 halves the KV cache footprint vs f16,
// fixing OOM on Gemma 4 31B dual-cache (SWA + global) at 32K context.
func (c *ModelConfig) EffectiveCacheTypeK() string {
	if c.CacheTypeK == "" {
		return "q8_0"
	}
	return c.CacheTypeK
}

// EffectiveCacheTypeV returns the KV cache quantization type for values,
// or "q8_0" if not set. See EffectiveCacheTypeK for rationale.
func (c *ModelConfig) EffectiveCacheTypeV() string {
	if c.CacheTypeV == "" {
		return "q8_0"
	}
	return c.CacheTypeV
}

// CalcAutoGPULayers calculates the number of GPU layers to offload based on
// available VRAM and model file size. Returns "999" (all layers) when the model
// fits with headroom, a reduced count when it doesn't, or "999" as fallback
// when VRAM info is unavailable.
func CalcAutoGPULayers(vramSizeKB uint64, modelFileSize int64) string {
	if vramSizeKB == 0 || modelFileSize <= 0 {
		return "999"
	}

	availGB := float64(vramSizeKB) / 1024.0 / 1024.0
	modelGB := float64(modelFileSize) / (1024.0 * 1024.0 * 1024.0)
	headroomGB := 4.0 // KV cache + SWA cache + runtime overhead + desktop compositor
	usableGB := availGB - headroomGB

	if usableGB <= 0 {
		return "1"
	}

	if modelGB <= usableGB {
		return "999"
	}

	safeLayers := int(math.Floor(999.0 * usableGB / modelGB))
	if safeLayers < 1 {
		safeLayers = 1
	}
	return strconv.Itoa(safeLayers)
}

// EffectiveKillTimeoutSec returns the configured kill timeout or 5 seconds.
func (c *ModelConfig) EffectiveKillTimeoutSec() int {
	if c.KillTimeoutSec == 0 {
		return 5
	}
	return c.KillTimeoutSec
}

// EffectiveHealthPollMs returns the configured health poll interval or 500ms.
func (c *ModelConfig) EffectiveHealthPollMs() int {
	if c.HealthPollMs == 0 {
		return 500
	}
	return c.HealthPollMs
}

// EffectiveHealthTimeoutMs returns the configured health timeout or 2000ms.
func (c *ModelConfig) EffectiveHealthTimeoutMs() int {
	if c.HealthTimeoutMs == 0 {
		return 2000
	}
	return c.HealthTimeoutMs
}

// EffectiveChatTimeoutSec returns the configured chat timeout or 10 seconds.
func (c *ModelConfig) EffectiveChatTimeoutSec() int {
	if c.ChatTimeoutSec == 0 {
		return 10
	}
	return c.ChatTimeoutSec
}

// EffectiveStartupTimeoutSec returns the configured startup timeout or 120 seconds.
func (c *ModelConfig) EffectiveStartupTimeoutSec() int {
	if c.StartupTimeoutSec == 0 {
		return 120
	}
	return c.StartupTimeoutSec
}

// EffectiveSpinnerIntervalMs returns the configured spinner interval or 100ms.
func (c *ModelConfig) EffectiveSpinnerIntervalMs() int {
	if c.SpinnerIntervalMs == 0 {
		return 100
	}
	return c.SpinnerIntervalMs
}

// EffectiveTokenBudgetPct returns the configured token budget percentage or 60.
func (c *ModelConfig) EffectiveTokenBudgetPct() int {
	if c.TokenBudgetPct == 0 {
		return 60
	}
	return c.TokenBudgetPct
}

// EffectiveHistoryTrimPct returns the configured history trim percentage or 80.
func (c *ModelConfig) EffectiveHistoryTrimPct() int {
	if c.HistoryTrimPct == 0 {
		return 80
	}
	return c.HistoryTrimPct
}

// EffectiveTokenCharDivisor returns the configured token/char divisor or 4.
func (c *ModelConfig) EffectiveTokenCharDivisor() int {
	if c.TokenCharDivisor == 0 {
		return 4
	}
	return c.TokenCharDivisor
}

// EffectiveHistoryLimit returns the configured history limit or 500.
func (c *ModelConfig) EffectiveHistoryLimit() int {
	if c.HistoryLimit == 0 {
		return 500
	}
	return c.HistoryLimit
}

// EffectiveTerminalWidthFallback returns the configured terminal width fallback or 80.
func (c *ModelConfig) EffectiveTerminalWidthFallback() int {
	if c.TerminalWidthFallback == 0 {
		return 80
	}
	return c.TerminalWidthFallback
}

// EffectiveStartupHistoryMax returns the configured startup history max or 5.
func (c *ModelConfig) EffectiveStartupHistoryMax() int {
	if c.StartupHistoryMax == 0 {
		return 5
	}
	return c.StartupHistoryMax
}

// EffectiveTaskNameTruncLen returns the configured task name truncation length or 40.
func (c *ModelConfig) EffectiveTaskNameTruncLen() int {
	if c.TaskNameTruncLen == 0 {
		return 40
	}
	return c.TaskNameTruncLen
}

// EffectiveSlugMaxLen returns the configured slug max length or 50.
func (c *ModelConfig) EffectiveSlugMaxLen() int {
	if c.SlugMaxLen == 0 {
		return 50
	}
	return c.SlugMaxLen
}

// EffectiveConfigDirName returns the configured config dir name or ".fakeoid".
func (c *ModelConfig) EffectiveConfigDirName() string {
	if c.ConfigDirName == "" {
		return ".fakeoid"
	}
	return c.ConfigDirName
}

// EffectiveModelSubdir returns the configured model subdirectory or "models".
func (c *ModelConfig) EffectiveModelSubdir() string {
	if c.ModelSubdir == "" {
		return "models"
	}
	return c.ModelSubdir
}

// EffectiveTaskSubdir returns the configured task subdirectory or "tasks".
func (c *ModelConfig) EffectiveTaskSubdir() string {
	if c.TaskSubdir == "" {
		return "tasks"
	}
	return c.TaskSubdir
}

// EffectiveHistoryFile returns the configured history filename or "history.json".
func (c *ModelConfig) EffectiveHistoryFile() string {
	if c.HistoryFile == "" {
		return "history.json"
	}
	return c.HistoryFile
}

// EffectiveConfigFile returns the configured config filename or "config.json".
func (c *ModelConfig) EffectiveConfigFile() string {
	if c.ConfigFile == "" {
		return "config.json"
	}
	return c.ConfigFile
}

// EffectiveTreeMaxDepth returns the configured tree max depth or 3.
func (c *ModelConfig) EffectiveTreeMaxDepth() int {
	if c.TreeMaxDepth == 0 {
		return 3
	}
	return c.TreeMaxDepth
}

// EffectiveTreeMaxLines returns the configured tree max lines or 200.
func (c *ModelConfig) EffectiveTreeMaxLines() int {
	if c.TreeMaxLines == 0 {
		return 200
	}
	return c.TreeMaxLines
}

// EffectiveTreeExcludes returns the configured tree excludes or the default list.
// A nil slice means "use defaults"; an empty non-nil slice means "exclude nothing".
func (c *ModelConfig) EffectiveTreeExcludes() []string {
	if c.TreeExcludes == nil {
		return []string{".git", "node_modules", "__pycache__", ".venv", "vendor", "build", "dist", ".fakeoid"}
	}
	return c.TreeExcludes
}

// EffectiveReadAllowPaths returns the configured read allow paths or the default list.
// A nil slice means "use defaults"; an empty non-nil slice means "no extra paths".
func (c *ModelConfig) EffectiveReadAllowPaths() []string {
	if c.ReadAllowPaths == nil {
		return []string{"/etc", "/proc"}
	}
	return c.ReadAllowPaths
}

// EffectiveAllowedBuildCommands returns the configured build commands or the default list.
// A nil slice means "use defaults"; an empty non-nil slice means "no commands allowed".
func (c *ModelConfig) EffectiveAllowedBuildCommands() []string {
	if c.AllowedBuildCommands == nil {
		return []string{"cargo", "rustc", "make", "cmake", "gcc", "g++", "clang", "clang++", "npm", "npx", "node", "go", "python", "python3", "pip"}
	}
	return c.AllowedBuildCommands
}

// EffectiveSSEBufferSize returns the configured SSE buffer size or 1MB.
func (c *ModelConfig) EffectiveSSEBufferSize() int {
	if c.SSEBufferSize == 0 {
		return 1024 * 1024
	}
	return c.SSEBufferSize
}

// EffectiveMaxIterations returns the configured max iterations or 100.
func (c *ModelConfig) EffectiveMaxIterations() int {
	if c.MaxIterations == 0 {
		return 100
	}
	return c.MaxIterations
}

// LoadConfig reads the config from the default location (~/.fakeoid/config.json).
func LoadConfig() (*ModelConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return &ModelConfig{}, nil
	}
	return LoadConfigFrom(filepath.Join(home, ".fakeoid", "config.json"))
}

// LoadConfigFrom reads a ModelConfig from the given path.
// Returns an empty config (no error) if the file does not exist.
func LoadConfigFrom(path string) (*ModelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &ModelConfig{}, nil
		}
		return nil, err
	}
	var cfg ModelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
