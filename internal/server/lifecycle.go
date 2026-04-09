package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/cmyster/fakeoid/internal/model"
)

// ServerConfig holds the configuration for starting a llama-server subprocess.
type ServerConfig struct {
	LlamaServerPath string
	ModelPath       string
	Port            int
	CtxSize         int
	GPULayers       string
	FlashAttn       string
	Host            string
	LogBufferMax    int
	KillTimeoutSec  int
	HealthPollMs    int
	HealthTimeoutMs int
	GPUComputePct  int
	TotalCUs       int
	GPUMaxAllocPct int
	VRAMSizeKB     uint64
	ModelFileSize  int64
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
		ctxSize = 16384
	}
	logBufMax := cfg.LogBufferMax
	if logBufMax == 0 {
		logBufMax = 200
	}
	return &Server{
		cfg:       cfg,
		port:      port,
		ctxSize:   ctxSize,
		logBuffer: NewLogBuffer(logBufMax),
		exitCh:    make(chan error, 1),
	}
}

// BuildCmd constructs the exec.Cmd for llama-server without starting it.
func (s *Server) BuildCmd() *exec.Cmd {
	gpuLayers := s.cfg.GPULayers
	if gpuLayers == "" {
		gpuLayers = model.CalcAutoGPULayers(s.cfg.VRAMSizeKB, s.cfg.ModelFileSize)
	}
	flashAttn := s.cfg.FlashAttn
	if flashAttn == "" {
		flashAttn = "on"
	}
	host := s.cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	cmd := exec.Command(s.cfg.LlamaServerPath,
		"--model", s.cfg.ModelPath,
		"--port", strconv.Itoa(s.port),
		"--ctx-size", strconv.Itoa(s.ctxSize),
		"--n-gpu-layers", gpuLayers,
		"--flash-attn", flashAttn,
		"--host", host,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Build custom env if any GPU-related env vars need injection
	needCUMask := s.cfg.GPUComputePct > 0 && s.cfg.GPUComputePct < 100 && s.cfg.TotalCUs > 0
	needAllocPct := s.cfg.GPUMaxAllocPct > 0 && s.cfg.GPUMaxAllocPct < 100
	if needCUMask || needAllocPct {
		cmd.Env = os.Environ()
		if needCUMask {
			cmd.Env = append(cmd.Env, "HSA_CU_MASK="+buildCUMask(s.cfg.GPUComputePct, s.cfg.TotalCUs))
		}
		if needAllocPct {
			cmd.Env = append(cmd.Env, fmt.Sprintf("GPU_MAX_ALLOC_PERCENT=%d", s.cfg.GPUMaxAllocPct))
		}
	}

	cmd.Stderr = s.logBuffer
	cmd.Stdout = s.logBuffer
	return cmd
}

// buildCUMask computes the HSA_CU_MASK value for the given percentage and total CUs.
// The enabled count is rounded down to the nearest even number for RDNA3 WGP compatibility.
// Returns a mask string in the format "0:0-N" where N is the last enabled CU index.
func buildCUMask(pct, totalCUs int) string {
	enabled := totalCUs * pct / 100
	// Round down to nearest even (WGP pairs for RDNA3)
	enabled = enabled &^ 1
	if enabled < 2 {
		enabled = 2
	}
	return fmt.Sprintf("0:0-%d", enabled-1)
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
	pollMs := s.cfg.HealthPollMs
	if pollMs == 0 {
		pollMs = 500
	}
	timeoutMs := s.cfg.HealthTimeoutMs
	if timeoutMs == 0 {
		timeoutMs = 2000
	}

	ticker := time.NewTicker(time.Duration(pollMs) * time.Millisecond)
	defer ticker.Stop()

	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
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

// Stop sends SIGTERM to the process group, waits for the configured timeout, then escalates to SIGKILL.
func (s *Server) Stop() error {
	if !s.started || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
	if err != nil {
		// Process may have already exited
		return nil
	}

	killTimeout := s.cfg.KillTimeoutSec
	if killTimeout == 0 {
		killTimeout = 5
	}

	// Send SIGTERM to process group
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	// Wait for clean exit
	select {
	case <-s.exitCh:
		return nil
	case <-time.After(time.Duration(killTimeout) * time.Second):
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
