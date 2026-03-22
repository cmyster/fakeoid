package model

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifySHA256Match(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "testfile")

	content := []byte("hello world")
	require.NoError(t, os.WriteFile(filePath, content, 0644))

	h := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(h[:])

	match, err := VerifySHA256(filePath, expectedHash)
	assert.NoError(t, err)
	assert.True(t, match, "SHA256 should match for correct hash")
}

func TestVerifySHA256Mismatch(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "testfile")

	content := []byte("hello world")
	require.NoError(t, os.WriteFile(filePath, content, 0644))

	match, err := VerifySHA256(filePath, "0000000000000000000000000000000000000000000000000000000000000000")
	assert.NoError(t, err)
	assert.False(t, match, "SHA256 should not match for wrong hash")
}

func TestVerifySHA256MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "nonexistent")

	_, err := VerifySHA256(filePath, "anyhash")
	assert.Error(t, err)
}
