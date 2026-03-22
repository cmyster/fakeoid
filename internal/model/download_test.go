package model

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRangeServer creates an httptest server that serves content and supports Range requests.
func newRangeServer(content string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			// Parse "bytes=N-"
			prefix := "bytes="
			if strings.HasPrefix(rangeHeader, prefix) {
				parts := strings.SplitN(rangeHeader[len(prefix):], "-", 2)
				offset, err := strconv.ParseInt(parts[0], 10, 64)
				if err == nil && offset < int64(len(content)) {
					w.Header().Set("Content-Length", strconv.Itoa(len(content)-int(offset)))
					w.WriteHeader(http.StatusPartialContent)
					w.Write([]byte(content[offset:]))
					return
				}
			}
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
}

func TestDownloadFreshFile(t *testing.T) {
	content := "hello model content for testing download"
	server := newRangeServer(content)
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "model.gguf")

	err := Download(server.URL, destPath, int64(len(content)), io.Discard, true)
	assert.NoError(t, err)

	data, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))

	// .part file should not remain
	_, err = os.Stat(destPath + ".part")
	assert.True(t, os.IsNotExist(err))
}

func TestDownloadResume(t *testing.T) {
	content := "abcdefghijklmnopqrstuvwxyz"
	server := newRangeServer(content)
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "model.gguf")
	partPath := destPath + ".part"

	// Create a .part file with first 10 bytes
	require.NoError(t, os.WriteFile(partPath, []byte(content[:10]), 0644))

	err := Download(server.URL, destPath, int64(len(content)), io.Discard, true)
	assert.NoError(t, err)

	data, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestDownloadServerResetOnResume(t *testing.T) {
	content := "full content from server"
	// Server that always returns 200 (ignores Range header)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "model.gguf")
	partPath := destPath + ".part"

	// Create partial .part file
	require.NoError(t, os.WriteFile(partPath, []byte("old partial data"), 0644))

	err := Download(server.URL, destPath, int64(len(content)), io.Discard, true)
	assert.NoError(t, err)

	data, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestDownloadNetworkError(t *testing.T) {
	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "model.gguf")
	partPath := destPath + ".part"

	// Create .part file to verify it is preserved
	require.NoError(t, os.WriteFile(partPath, []byte("partial data"), 0644))

	err := Download("http://127.0.0.1:1", destPath, 1000, io.Discard, true)
	assert.Error(t, err)

	// .part file should be preserved
	_, statErr := os.Stat(partPath)
	assert.NoError(t, statErr, ".part file should be preserved on network error")
}

func TestDownloadHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "model.gguf")

	err := Download(server.URL, destPath, 1000, io.Discard, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}
