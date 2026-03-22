package model

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	// DefaultModelRepo is the HuggingFace repository for the default model.
	DefaultModelRepo = "bartowski/Qwen2.5-Coder-32B-Instruct-GGUF"

	// DefaultModelFile is the filename of the default GGUF model.
	DefaultModelFile = "Qwen2.5-Coder-32B-Instruct-Q4_K_M.gguf"

	// DefaultModelURL is the direct download URL for the default model.
	DefaultModelURL = "https://huggingface.co/bartowski/Qwen2.5-Coder-32B-Instruct-GGUF/resolve/main/Qwen2.5-Coder-32B-Instruct-Q4_K_M.gguf"

	// DefaultModelHash is the expected SHA256 hash of the default model file.
	DefaultModelHash = "8e2fd78ff55e7cdf577fda257bac2776feb7d73d922613caf35468073807e815"

	// DefaultModelSize is the expected file size in bytes (~19.85 GB).
	DefaultModelSize = int64(19_900_000_000)

	// DefaultModelName is the human-readable name of the default model.
	DefaultModelName = "Qwen2.5-Coder-32B-Q4_K_M"
)

// ModelConfig holds user configuration overrides loaded from config.json.
type ModelConfig struct {
	ModelPath string `json:"model_path"`
	Port      int    `json:"port"`
	CtxSize   int    `json:"ctx_size"`
}

// EffectivePort returns the configured port, or 8080 if not set.
func (c *ModelConfig) EffectivePort() int {
	if c.Port == 0 {
		return 8080
	}
	return c.Port
}

// EffectiveCtxSize returns the configured context size, or 16384 if not set.
// The Qwen2.5-Coder-32B model uses ~18.5GB VRAM, leaving ~5.5GB for KV cache
// on a 24GB card. 16K context needs ~4GB KV cache, fitting with headroom.
// 32K would need ~8GB and OOM. Override via config.json ctx_size if needed.
func (c *ModelConfig) EffectiveCtxSize() int {
	if c.CtxSize == 0 {
		return 16384
	}
	return c.CtxSize
}

// DefaultModelPath returns the default filesystem path for the cached model.
func DefaultModelPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".fakeoid", "models", DefaultModelFile)
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
