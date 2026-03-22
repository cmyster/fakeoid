package server

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// ServerConfig holds the configuration for starting a llama-server subprocess.
type ServerConfig struct {
	LlamaServerPath string
	ModelPath       string
	Port            int
	CtxSize         int
}

// Server manages a llama-server subprocess.
type Server struct {
	cfg       ServerConfig
	cmd       *exec.Cmd
	port      int
	ctxSize   int
	logBuffer *LogBuffer
	exitCh    chan error
	started   bool

	// CmdFactory allows tests to intercept command construction.
	// If nil, BuildCmd is used.
	CmdFactory func() *exec.Cmd
}

// NewServer creates a new Server with the given configuration.
func NewServer(cfg ServerConfig) *Server {
	port := cfg.Port
	if port == 0 {
		port = 8080
	}
	ctxSize := cfg.CtxSize
	if ctxSize == 0 {
		ctxSize = 8192
	}
	return &Server{
		cfg:       cfg,
		port:      port,
		ctxSize:   ctxSize,
		logBuffer: NewLogBuffer(200),
		exitCh:    make(chan error, 1),
	}
}

// BuildCmd constructs the exec.Cmd for llama-server without starting it.
func (s *Server) BuildCmd() *exec.Cmd {
	cmd := exec.Command(s.cfg.LlamaServerPath,
		"--model", s.cfg.ModelPath,
		"--port", strconv.Itoa(s.port),
		"--ctx-size", strconv.Itoa(s.ctxSize),
		"--n-gpu-layers", "999",
		"--flash-attn", "on",
		"--host", "127.0.0.1",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stderr = s.logBuffer
	cmd.Stdout = s.logBuffer
	return cmd
}

// Start starts the llama-server subprocess.
func (s *Server) Start(ctx context.Context) error {
	if err := killProcessOnPort(s.port); err != nil {
		// Best-effort port cleanup, log but continue
	}

	var cmd *exec.Cmd
	if s.CmdFactory != nil {
		cmd = s.CmdFactory()
	} else {
		cmd = s.BuildCmd()
	}
	s.cmd = cmd

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start llama-server: %w", err)
	}
	s.started = true

	go func() {
		s.exitCh <- s.cmd.Wait()
	}()

	return nil
}

// WaitHealthy polls the /health endpoint until 200 OK, context cancellation,
// or subprocess crash.
func (s *Server) WaitHealthy(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", s.port)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-s.exitCh:
			return fmt.Errorf("llama-server exited during startup: %w", err)
		case <-ticker.C:
			resp, err := client.Get(healthURL)
			if err != nil {
				continue // connection refused = not ready yet
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			// 503 = still loading, keep polling
		}
	}
}

// Stop sends SIGTERM to the process group, waits 5s, then escalates to SIGKILL.
func (s *Server) Stop() error {
	if !s.started || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
	if err != nil {
		// Process may have already exited
		return nil
	}

	// Send SIGTERM to process group
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	// Wait up to 5 seconds for clean exit
	select {
	case <-s.exitCh:
		return nil
	case <-time.After(5 * time.Second):
		// Escalate to SIGKILL
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-s.exitCh
		return nil
	}
}

// LogDump returns the captured log lines for error reporting.
func (s *Server) LogDump() []string {
	return s.logBuffer.Dump()
}
