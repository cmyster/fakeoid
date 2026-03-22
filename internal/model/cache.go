package model

import (
	"os"
	"path/filepath"
)

// EnsureCacheDir creates the default cache directory (~/.fakeoid/models/) if it
// does not exist and returns its path.
func EnsureCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(home, ".fakeoid", "models")
	return EnsureCacheDirAt(cacheDir)
}

// EnsureCacheDirAt creates the given directory if it does not exist and returns its path.
func EnsureCacheDirAt(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// CachedModelInfo checks for a cached model at the default location,
// respecting config file overrides.
func CachedModelInfo(configPath string) (name string, sizeBytes int64, exists bool, err error) {
	return CachedModelInfoAt(DefaultModelPath(), configPath)
}

// CachedModelInfoAt checks for a cached model. If configPath is non-empty and
// the config has a model_path override, that path is checked instead of defaultPath.
func CachedModelInfoAt(defaultPath, configPath string) (name string, sizeBytes int64, exists bool, err error) {
	modelPath := defaultPath

	// Check config override
	if configPath != "" {
		cfg, cfgErr := LoadConfigFrom(configPath)
		if cfgErr != nil {
			return "", 0, false, cfgErr
		}
		if cfg.ModelPath != "" {
			modelPath = cfg.ModelPath
		}
	}

	info, err := os.Stat(modelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0, false, nil
		}
		return "", 0, false, err
	}

	return filepath.Base(modelPath), info.Size(), true, nil
}
