package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             9090,
		CtxSize:          16384,
	}
	s := NewServer(cfg)
	require.NotNil(t, s)
	assert.Equal(t, 9090, s.port)
	assert.NotNil(t, s.logBuffer)
	assert.NotNil(t, s.exitCh)
}

func TestNewServerDefaultPort(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
	}
	s := NewServer(cfg)
	assert.Equal(t, 8080, s.port)
}

func TestNewServerDefaultCtxSize(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
	}
	s := NewServer(cfg)
	assert.Equal(t, 16384, s.ctxSize)
}

func TestLifecycleBuildCmd(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/opt/llama/llama-server",
		ModelPath:        "/models/qwen.gguf",
		Port:             8080,
		CtxSize:          8192,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()

	assert.Equal(t, "/opt/llama/llama-server", cmd.Path)

	args := strings.Join(cmd.Args[1:], " ")
	assert.Contains(t, args, "--model /models/qwen.gguf")
	assert.Contains(t, args, "--port 8080")
	assert.Contains(t, args, "--ctx-size 8192")
	assert.Contains(t, args, "--n-gpu-layers 999")
	assert.Contains(t, args, "--flash-attn on")
	assert.Contains(t, args, "--host 127.0.0.1")

	// Verify process group setup
	require.NotNil(t, cmd.SysProcAttr)
	assert.True(t, cmd.SysProcAttr.Setpgid)

	// Verify stderr/stdout go to log buffer
	assert.Equal(t, s.logBuffer, cmd.Stderr)
	assert.Equal(t, s.logBuffer, cmd.Stdout)
}

func TestLifecycleBuildCmdCustomPort(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             9090,
		CtxSize:          16384,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()

	args := strings.Join(cmd.Args[1:], " ")
	assert.Contains(t, args, "--port 9090")
	assert.Contains(t, args, "--ctx-size 16384")
}

func TestWaitHealthySuccess(t *testing.T) {
	// Mock server that returns 503 twice, then 200
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())

	s := &Server{
		port:      port,
		logBuffer: NewLogBuffer(200),
		exitCh:    make(chan error, 1),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.WaitHealthy(ctx)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, int(calls.Load()), 3)
}

func TestWaitHealthyTimeout(t *testing.T) {
	// Server that always returns 503
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())

	s := &Server{
		port:      port,
		logBuffer: NewLogBuffer(200),
		exitCh:    make(chan error, 1),
	}

	// Use a very short context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	err := s.WaitHealthy(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestWaitHealthyCrashDetection(t *testing.T) {
	// Server that never becomes healthy
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())

	s := &Server{
		port:      port,
		logBuffer: NewLogBuffer(200),
		exitCh:    make(chan error, 1),
	}

	// Simulate crash by sending to exitCh
	go func() {
		time.Sleep(200 * time.Millisecond)
		s.exitCh <- fmt.Errorf("exit status 1")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := s.WaitHealthy(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exited during startup")
}

func TestWaitHealthyConnectionRefused(t *testing.T) {
	// Use a port that nothing is listening on
	s := &Server{
		port:      59999,
		logBuffer: NewLogBuffer(200),
		exitCh:    make(chan error, 1),
	}

	// Short timeout -- connection refused should be treated as "not ready yet"
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	err := s.WaitHealthy(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestLogDump(t *testing.T) {
	s := &Server{
		logBuffer: NewLogBuffer(200),
	}
	s.logBuffer.Write([]byte("line1\nline2\nline3\n"))
	lines := s.LogDump()
	assert.Equal(t, []string{"line1", "line2", "line3"}, lines)
}

func TestBuildCUMask(t *testing.T) {
	tests := []struct {
		name     string
		pct      int
		totalCUs int
		expected string
	}{
		{"75pct of 96 CUs", 75, 96, "0:0-71"},
		{"50pct of 96 CUs", 50, 96, "0:0-47"},
		{"75pct of 60 CUs (round to even)", 75, 60, "0:0-43"},
		{"25pct of 96 CUs", 25, 96, "0:0-23"},
		{"10pct of 96 CUs", 10, 96, "0:0-7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCUMask(tt.pct, tt.totalCUs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildCmdWithCUMask(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPUComputePct:    75,
		TotalCUs:         96,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()

	require.NotNil(t, cmd.Env)
	assert.Greater(t, len(cmd.Env), 1, "should include inherited env vars")

	found := false
	for _, env := range cmd.Env {
		if env == "HSA_CU_MASK=0:0-71" {
			found = true
			break
		}
	}
	assert.True(t, found, "HSA_CU_MASK=0:0-71 should be in cmd.Env")
}

func TestBuildCmdNoCUMask(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPUComputePct:    0,
		TotalCUs:         96,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()
	assert.Nil(t, cmd.Env, "cmd.Env should be nil when GPUComputePct is 0 (default)")
}

func TestBuildCmdNoCUMaskAt100(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPUComputePct:    100,
		TotalCUs:         96,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()
	assert.Nil(t, cmd.Env, "cmd.Env should be nil when GPUComputePct is 100")
}

func TestBuildCmdNoCUMaskZeroCUs(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPUComputePct:    75,
		TotalCUs:         0,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()
	assert.Nil(t, cmd.Env, "cmd.Env should be nil when TotalCUs is 0 (unknown)")
}

func TestBuildCmdWithGPUMaxAllocPct(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPUMaxAllocPct:   80,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()

	require.NotNil(t, cmd.Env)
	found := false
	for _, env := range cmd.Env {
		if env == "GPU_MAX_ALLOC_PERCENT=80" {
			found = true
			break
		}
	}
	assert.True(t, found, "GPU_MAX_ALLOC_PERCENT=80 should be in cmd.Env")
}

func TestBuildCmdNoGPUMaxAllocPctDefault(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPUMaxAllocPct:   0,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()
	// No custom env when GPUMaxAllocPct is 0 (default)
	for _, env := range cmd.Env {
		assert.NotContains(t, env, "GPU_MAX_ALLOC_PERCENT", "should not inject GPU_MAX_ALLOC_PERCENT when 0")
	}
}

func TestBuildCmdNoGPUMaxAllocPctAt100(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPUMaxAllocPct:   100,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()
	for _, env := range cmd.Env {
		assert.NotContains(t, env, "GPU_MAX_ALLOC_PERCENT", "should not inject GPU_MAX_ALLOC_PERCENT at 100")
	}
}

func TestBuildCmdBothEnvVars(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPUComputePct:    75,
		TotalCUs:         96,
		GPUMaxAllocPct:   80,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()

	require.NotNil(t, cmd.Env)
	foundCUMask := false
	foundAllocPct := false
	for _, env := range cmd.Env {
		if env == "HSA_CU_MASK=0:0-71" {
			foundCUMask = true
		}
		if env == "GPU_MAX_ALLOC_PERCENT=80" {
			foundAllocPct = true
		}
	}
	assert.True(t, foundCUMask, "HSA_CU_MASK=0:0-71 should be in cmd.Env")
	assert.True(t, foundAllocPct, "GPU_MAX_ALLOC_PERCENT=80 should be in cmd.Env")
}

func TestBuildCmdAutoGPULayers(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPULayers:        "",
		VRAMSizeKB:       16777216, // 16GB
		ModelFileSize:    19_900_000_000,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()

	args := strings.Join(cmd.Args[1:], " ")
	assert.NotContains(t, args, "--n-gpu-layers 999", "16GB VRAM should auto-calc fewer than 999 layers")
	assert.Contains(t, args, "--n-gpu-layers", "should still have --n-gpu-layers arg")
}

func TestBuildCmdAutoGPULayersFallback(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPULayers:        "",
		VRAMSizeKB:       0,
		ModelFileSize:    19_900_000_000,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()

	args := strings.Join(cmd.Args[1:], " ")
	assert.Contains(t, args, "--n-gpu-layers 999", "should fallback to 999 when VRAM unknown")
}

func TestBuildCmdGPULayersUserOverride(t *testing.T) {
	cfg := ServerConfig{
		LlamaServerPath: "/usr/bin/llama-server",
		ModelPath:        "/models/test.gguf",
		Port:             8080,
		CtxSize:          8192,
		GPULayers:        "30",
		VRAMSizeKB:       16777216,
		ModelFileSize:    19_900_000_000,
	}
	s := NewServer(cfg)
	cmd := s.BuildCmd()

	args := strings.Join(cmd.Args[1:], " ")
	assert.Contains(t, args, "--n-gpu-layers 30", "user override should take priority over auto-calc")
}

func TestShutdownSIGTERM(t *testing.T) {
	// Use a real subprocess (sleep) to test Stop
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	s := &Server{
		logBuffer: NewLogBuffer(200),
		exitCh:    make(chan error, 1),
	}

	err := cmd.Start()
	require.NoError(t, err)
	s.cmd = cmd
	s.started = true

	// Monitor exit in goroutine like real Start would
	go func() {
		s.exitCh <- s.cmd.Wait()
	}()

	// Give the goroutine a moment to start
	time.Sleep(50 * time.Millisecond)

	err = s.Stop()
	assert.NoError(t, err)
}
