package validate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRunner implements CommandRunner for testing.
type mockRunner struct {
	lookPathFunc func(file string) (string, error)
	runFunc      func(name string, args ...string) ([]byte, error)
}

func (m *mockRunner) LookPath(file string) (string, error) {
	return m.lookPathFunc(file)
}

func (m *mockRunner) Run(name string, args ...string) ([]byte, error) {
	return m.runFunc(name, args...)
}

func TestCheckLlamaServer_Found(t *testing.T) {
	runner := &mockRunner{
		lookPathFunc: func(file string) (string, error) {
			assert.Equal(t, "llama-server", file)
			return "/usr/bin/llama-server", nil
		},
	}

	result, err := CheckLlamaServer(runner, true)
	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Equal(t, "llama-server", result.Name)
	assert.Contains(t, result.Detail, "/usr/bin/llama-server")
	assert.Empty(t, result.Error)
}

func TestCheckLlamaServer_NotFound(t *testing.T) {
	runner := &mockRunner{
		lookPathFunc: func(file string) (string, error) {
			return "", errors.New("not found")
		},
	}

	result, err := CheckLlamaServer(runner, false)
	require.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Equal(t, "llama-server not found in PATH", result.Error)
}

func TestCheckROCmBuild_Linked(t *testing.T) {
	lddOutput := `	linux-vdso.so.1 (0x00007fffc9bfe000)
	libamdhip64.so.7 => /usr/lib64/libamdhip64.so.7 (0x00007f5d3c000000)
	libhsa-runtime64.so.1 => /usr/lib64/libhsa-runtime64.so.1 (0x00007f5d3b800000)
	libc.so.6 => /lib64/libc.so.6 (0x00007f5d3b600000)
`
	runner := &mockRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			assert.Equal(t, "ldd", name)
			return []byte(lddOutput), nil
		},
	}

	result, err := CheckROCmBuild(runner, "/usr/bin/llama-server", true)
	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Equal(t, "rocm-build", result.Name)
	assert.Contains(t, result.Detail, "libamdhip64")
}

func TestCheckROCmBuild_NotLinked(t *testing.T) {
	lddOutput := `	linux-vdso.so.1 (0x00007fffc9bfe000)
	libstdc++.so.6 => /usr/lib64/libstdc++.so.6 (0x00007f5d3c000000)
	libc.so.6 => /lib64/libc.so.6 (0x00007f5d3b600000)
`
	runner := &mockRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return []byte(lddOutput), nil
		},
	}

	result, err := CheckROCmBuild(runner, "/usr/bin/llama-server", false)
	require.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Error, "not built with ROCm support")
	assert.Contains(t, result.Error, "libamdhip64 not linked")
}

func TestCheckROCmBuild_LddError(t *testing.T) {
	runner := &mockRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return nil, errors.New("ldd failed")
		},
	}

	result, err := CheckROCmBuild(runner, "/usr/bin/llama-server", false)
	require.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Error, "failed to run ldd")
}
