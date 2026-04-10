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

// CachedModelInfo checks for a cached model at the path resolved from cfg.
func CachedModelInfo(cfg *ModelConfig) (name string, sizeBytes int64, exists bool, err error) {
	modelPath := cfg.EffectiveModelPath()
	if modelPath == "" {
		return "", 0, false, nil
	}
	return CachedModelInfoAt(modelPath)
}

// CachedModelInfoAt checks for a cached model at the given path.
func CachedModelInfoAt(modelPath string) (name string, sizeBytes int64, exists bool, err error) {
	info, err := os.Stat(modelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0, false, nil
		}
		return "", 0, false, err
	}

	return filepath.Base(modelPath), info.Size(), true, nil
}
