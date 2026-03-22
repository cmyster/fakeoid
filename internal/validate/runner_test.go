package validate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAll_AllPass(t *testing.T) {
	runner := &mockRunner{
		lookPathFunc: func(file string) (string, error) {
			switch file {
			case "llama-server":
				return "/usr/bin/llama-server", nil
			case "rocminfo":
				return "/usr/bin/rocminfo", nil
			}
			return "", errors.New("not found")
		},
		runFunc: func(name string, args ...string) ([]byte, error) {
			switch name {
			case "ldd":
				return []byte("libamdhip64.so.7 => /usr/lib64/libamdhip64.so.7\n"), nil
			case "rocminfo":
				return []byte(rocmInfoGPUOutput), nil
			}
			return nil, errors.New("unknown command")
		},
	}

	results, err := RunAll(runner, false)
	require.NoError(t, err)
	assert.Len(t, results, 4)
	for _, r := range results {
		assert.True(t, r.Passed, "check %s should pass", r.Name)
	}
}

func TestRunAll_FailFast(t *testing.T) {
	runner := &mockRunner{
		lookPathFunc: func(file string) (string, error) {
			if file == "llama-server" {
				return "/usr/bin/llama-server", nil
			}
			return "", errors.New("not found")
		},
		runFunc: func(name string, args ...string) ([]byte, error) {
			// ldd output without libamdhip64 -> second check fails
			return []byte("libc.so.6 => /lib64/libc.so.6\n"), nil
		},
	}

	results, err := RunAll(runner, false)
	require.NoError(t, err)
	// Should stop after 2 results (llama-server pass, rocm-build fail)
	assert.Len(t, results, 2)
	assert.True(t, results[0].Passed)
	assert.False(t, results[1].Passed)
}

func TestRunAll_Verbose(t *testing.T) {
	runner := &mockRunner{
		lookPathFunc: func(file string) (string, error) {
			switch file {
			case "llama-server":
				return "/usr/bin/llama-server", nil
			case "rocminfo":
				return "/usr/bin/rocminfo", nil
			}
			return "", errors.New("not found")
		},
		runFunc: func(name string, args ...string) ([]byte, error) {
			switch name {
			case "ldd":
				return []byte("libamdhip64.so.7 => /usr/lib64/libamdhip64.so.7\n"), nil
			case "rocminfo":
				return []byte(rocmInfoGPUOutput), nil
			}
			return nil, errors.New("unknown command")
		},
	}

	results, err := RunAll(runner, true)
	require.NoError(t, err)
	assert.Len(t, results, 4)
	// Verbose mode should populate Detail on passing checks
	for _, r := range results {
		assert.NotEmpty(t, r.Detail, "verbose check %s should have detail", r.Name)
	}
}

func TestRunAll_FirstCheckFails(t *testing.T) {
	runner := &mockRunner{
		lookPathFunc: func(file string) (string, error) {
			return "", errors.New("not found")
		},
	}

	results, err := RunAll(runner, false)
	require.NoError(t, err)
	// Should stop after first check
	assert.Len(t, results, 1)
	assert.False(t, results[0].Passed)
	assert.Equal(t, "llama-server not found in PATH", results[0].Error)
}
