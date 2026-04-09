package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cmyster/fakeoid/internal/agent"
	"github.com/cmyster/fakeoid/internal/model"
	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/cmyster/fakeoid/internal/server"
	"github.com/cmyster/fakeoid/internal/shell"
	"github.com/cmyster/fakeoid/internal/state"
	"github.com/cmyster/fakeoid/internal/validate"
	"github.com/spf13/cobra"
)

// srv holds the running llama-server instance for subcommand access and cleanup.
var srv *server.Server

// srvPort holds the effective port for the running server.
var srvPort int

// gpuName is cached during PersistentPreRunE for the shell banner.
var gpuName string

// cfg holds the loaded configuration, shared between PersistentPreRunE and RunE.
var cfg *model.ModelConfig

var rootCmd = &cobra.Command{
	Use:   "fakeoid",
	Short: "Local AI coding assistant",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip validation for commands that don't need it.
		name := cmd.Name()
		if name == "check" || name == "download" || name == "help" || name == "completion" || name == "clear-history" {
			return nil
		}

		results, err := validate.RunAll(&validate.ExecRunner{}, false)
		if err != nil {
			return fmt.Errorf("%s", err)
		}
		for _, r := range results {
			if !r.Passed {
				return fmt.Errorf("%s", r.Error)
			}
		}
		// Cache GPU name and CU count for shell banner and compute throttling
		gpuName = "unknown"
		var totalCUs int
		var vramSizeKB uint64
		if gpuResult, gpus, _ := validate.CheckGPU(&validate.ExecRunner{}, false); gpuResult != nil && gpuResult.Passed && len(gpus) > 0 {
			gpuName = gpus[0].Name
			totalCUs = gpus[0].ComputeUnits
			vramSizeKB = gpus[0].VRAMSizeKB
		}

		// Auto-download model if not cached
		modelPath := model.DefaultModelPath()
		if _, err := os.Stat(modelPath); os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "No model found. Downloading default...")
			if err := model.DownloadDefault(os.Stderr); err != nil {
				return fmt.Errorf("%s", err)
			}
		}

		// --- Server lifecycle startup ---

		// Load config for all overrides
		cfg, _ = model.LoadConfig()

		// Resolve llama-server path (already validated by RunAll above)
		llamaPath, err := exec.LookPath("llama-server")
		if err != nil {
			return fmt.Errorf("llama-server not found in PATH")
		}

		// Resolve model path: config override or default
		if cfg.ModelPath != "" {
			modelPath = cfg.ModelPath
		}

		srvPort = cfg.EffectivePort()

		srv = server.NewServer(server.ServerConfig{
			LlamaServerPath: llamaPath,
			ModelPath:       modelPath,
			Port:            cfg.EffectivePort(),
			CtxSize:         cfg.EffectiveCtxSize(),
			GPULayers:       cfg.GPULayers,
			FlashAttn:       cfg.EffectiveFlashAttn(),
			Host:            cfg.EffectiveHost(),
			LogBufferMax:    cfg.EffectiveLogBufferMax(),
			KillTimeoutSec:  cfg.EffectiveKillTimeoutSec(),
			HealthPollMs:    cfg.EffectiveHealthPollMs(),
			HealthTimeoutMs: cfg.EffectiveHealthTimeoutMs(),
			GPUComputePct:  cfg.EffectiveGPUComputePct(),
			TotalCUs:       totalCUs,
			GPUMaxAllocPct: cfg.EffectiveGPUMaxAllocPct(),
			VRAMSizeKB:     vramSizeKB,
			ModelFileSize:  cfg.EffectiveModelSize(),
		})

		startupTimeout := time.Duration(cfg.EffectiveStartupTimeoutSec()) * time.Second

		// Create a context with timeout for health check
		healthCtx, healthCancel := context.WithTimeout(cmd.Context(), startupTimeout)
		defer healthCancel()

		// Create spinner
		sp := spinner.New(spinner.CharSets[14], time.Duration(cfg.EffectiveSpinnerIntervalMs())*time.Millisecond)
		sp.Suffix = " Starting llama-server..."
		sp.Writer = os.Stderr
		sp.Start()

		if err := srv.Start(healthCtx); err != nil {
			sp.Stop()
			return fmt.Errorf("failed to start llama-server: %s", err)
		}

		sp.Suffix = " Loading model..."

		if err := srv.WaitHealthy(healthCtx); err != nil {
			sp.Stop()
			// Determine error type
			if healthCtx.Err() == context.DeadlineExceeded {
				fmt.Fprintf(os.Stderr, "error: llama-server did not become healthy within %ds\n", cfg.EffectiveStartupTimeoutSec())
			} else if cmd.Context().Err() != nil {
				// Signal received (Ctrl+C) — cleanup handled by Execute() goroutine
				fmt.Fprintf(os.Stderr, "\n")
				return cmd.Context().Err()
			} else {
				fmt.Fprintf(os.Stderr, "error: llama-server crashed during startup\n")
			}
			// Print last log lines
			lines := srv.LogDump()
			if len(lines) > 0 {
				fmt.Fprintf(os.Stderr, "Last server output:\n")
				for _, line := range lines {
					fmt.Fprintf(os.Stderr, "  %s\n", line)
				}
			}
			// Clean up the process
			_ = srv.Stop()
			return fmt.Errorf("server startup failed")
		}

		sp.Stop()

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg == nil {
			cfg, _ = model.LoadConfig()
		}

		// Build history path
		home, _ := os.UserHomeDir()
		historyDir := filepath.Join(home, cfg.EffectiveConfigDirName())
		if err := os.MkdirAll(historyDir, 0o755); err != nil {
			return fmt.Errorf("create history directory: %s", err)
		}
		historyPath := filepath.Join(historyDir, "history")

		// Create Agent 1 and wire into runner
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %s", err)
		}

		sb, err := sandbox.New(cwd, cfg.EffectiveReadAllowPaths())
		if err != nil {
			return fmt.Errorf("initialize sandbox: %s", err)
		}
		defer sb.Close()

		// Set up state directory for task history persistence
		stateDir := filepath.Join(cwd, cfg.EffectiveConfigDirName())
		if err := state.EnsureGitignore(sb); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not update .gitignore: %s\n", err)
		}
		taskSubpath := filepath.Join(cfg.EffectiveConfigDirName(), cfg.EffectiveTaskSubdir())
		if err := sb.MkdirAll(taskSubpath, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create state directory: %s\n", err)
		}

		taskDir := filepath.Join(cwd, cfg.EffectiveConfigDirName(), cfg.EffectiveTaskSubdir())
		runner := agent.NewAgentRunner(taskDir)
		a1 := agent.NewAgent1(cwd, taskDir)
		runner.ActivateAgent(a1)

		client := server.NewClient(srvPort, cfg.EffectiveChatTimeoutSec(), cfg.EffectiveSSEBufferSize())
		sh, err := shell.New(client, cfg.EffectiveCtxSize(), cfg.EffectiveModelName(), gpuName, historyPath,
			shell.WithAgentRunner(runner),
			shell.WithCWD(cwd),
			shell.WithSandbox(sb),
			shell.WithStateDir(stateDir),
			shell.WithHistoryLimit(cfg.EffectiveHistoryLimit()),
			shell.WithStartupHistoryMax(cfg.EffectiveStartupHistoryMax()),
			shell.WithHistoryTrimPct(cfg.EffectiveHistoryTrimPct()),
			shell.WithTerminalWidthFallback(cfg.EffectiveTerminalWidthFallback()),
			shell.WithTaskNameTruncLen(cfg.EffectiveTaskNameTruncLen()),
			shell.WithTokenBudgetPct(cfg.EffectiveTokenBudgetPct()),
			shell.WithTokenCharDivisor(cfg.EffectiveTokenCharDivisor()),
			shell.WithMaxIterations(cfg.EffectiveMaxIterations()),
			shell.WithSlugMaxLen(cfg.EffectiveSlugMaxLen()),
			shell.WithTreeMaxDepth(cfg.EffectiveTreeMaxDepth()),
			shell.WithTreeMaxLines(cfg.EffectiveTreeMaxLines()),
			shell.WithTreeExcludes(cfg.EffectiveTreeExcludes()),
			shell.WithHistoryFile(cfg.EffectiveHistoryFile()),
		)
		if err != nil {
			return fmt.Errorf("failed to initialize shell: %s", err)
		}
		defer sh.Close()

		return sh.Run(cmd.Context())
	},
}

// Execute runs the root command with signal handling for graceful shutdown.
func Execute() error {
	// Set up signal-aware context
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run the command
	err := rootCmd.ExecuteContext(ctx)

	// Always clean up the server on exit, regardless of how we got here
	if srv != nil {
		fmt.Fprintln(os.Stderr, "Shutting down...")
		_ = srv.Stop()
	}

	return err
}
