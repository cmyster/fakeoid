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
	assert.Equal(t, 8192, s.ctxSize)
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
