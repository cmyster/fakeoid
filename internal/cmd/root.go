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
		// Cache GPU name for shell banner
		gpuName = "unknown"
		if gpuResult, gpus, _ := validate.CheckGPU(&validate.ExecRunner{}, false); gpuResult != nil && gpuResult.Passed && len(gpus) > 0 {
			gpuName = gpus[0].Name
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

		// Load config for port/ctx-size overrides
		cfg, _ := model.LoadConfig()

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
		})

		// Create a context with timeout for health check (120s)
		healthCtx, healthCancel := context.WithTimeout(cmd.Context(), 120*time.Second)
		defer healthCancel()

		// Create spinner
		sp := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
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
				fmt.Fprintf(os.Stderr, "error: llama-server did not become healthy within 120s\n")
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
		cfg, _ := model.LoadConfig()

		// Build history path
		home, _ := os.UserHomeDir()
		historyDir := filepath.Join(home, ".fakeoid")
		if err := os.MkdirAll(historyDir, 0o755); err != nil {
			return fmt.Errorf("create history directory: %s", err)
		}
		historyPath := filepath.Join(historyDir, "history")

		// Create Agent 1 and wire into runner
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %s", err)
		}

		sb, err := sandbox.New(cwd)
		if err != nil {
			return fmt.Errorf("initialize sandbox: %s", err)
		}
		defer sb.Close()

		// Set up state directory for task history persistence
		stateDir := filepath.Join(cwd, ".fakeoid")
		if err := state.EnsureGitignore(sb); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not update .gitignore: %s\n", err)
		}
		if err := sb.MkdirAll(".fakeoid/tasks", 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create state directory: %s\n", err)
		}

		taskDir := filepath.Join(cwd, ".fakeoid", "tasks")
		runner := agent.NewAgentRunner(taskDir)
		a1 := agent.NewAgent1(cwd, taskDir)
		runner.ActivateAgent(a1)

		client := server.NewClient(srvPort)
		sh, err := shell.New(client, cfg.EffectiveCtxSize(), model.DefaultModelName, gpuName, historyPath,
			shell.WithAgentRunner(runner),
			shell.WithCWD(cwd),
			shell.WithSandbox(sb),
			shell.WithStateDir(stateDir),
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
