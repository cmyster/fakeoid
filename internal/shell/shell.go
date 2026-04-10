package shell

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"time"

	"github.com/chzyer/readline"
	"github.com/cmyster/fakeoid/internal/agent"
	"golang.org/x/term"
	"github.com/cmyster/fakeoid/internal/extract"
	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/cmyster/fakeoid/internal/server"
	"github.com/cmyster/fakeoid/internal/state"
)

// ChatClient defines the interface for LLM communication, enabling test mocks.
type ChatClient interface {
	StreamChatCompletion(ctx context.Context, messages []server.Message, onToken func(string)) (server.StreamResult, error)
}

// Shell implements the interactive REPL for conversational LLM interaction.
type Shell struct {
	rl          *readline.Instance
	client      ChatClient
	runner      *agent.AgentRunner
	history     []server.Message // fallback when runner is nil (backward compat)
	ctxSize     int
	usedTokens  int
	modelName   string
	gpuName     string
	cwd         string
	sandbox     *sandbox.Sandbox
	stateDir    string // absolute path to .fakeoid/ state directory
	stdout      io.Writer
	stderr      io.Writer
	lastReadEOF bool // set when EOF encountered during multi-line continuation

	// Configurable values (set via options, zero = use hardcoded defaults)
	historyLimit          int
	startupHistoryMax     int
	historyTrimPct        int
	terminalWidthFallback int
	taskNameTruncLen      int
	tokenBudgetPct        int
	tokenCharDivisor      int
	maxIterations         int
	buildVerification     bool
	slugMaxLen            int
	treeMaxDepth          int
	treeMaxLines          int
	treeExcludes          []string
	historyFile           string
}

// Option configures the Shell for testing or customization.
type Option func(*shellConfig)

type shellConfig struct {
	stdin    io.ReadCloser
	stdout   io.Writer
	stderr   io.Writer
	runner   *agent.AgentRunner
	cwd      string
	sandbox  *sandbox.Sandbox
	stateDir string

	// Configurable values
	historyLimit          int
	startupHistoryMax     int
	historyTrimPct        int
	terminalWidthFallback int
	taskNameTruncLen      int
	tokenBudgetPct        int
	tokenCharDivisor      int
	maxIterations         int
	buildVerification     *bool // nil = default (true)
	slugMaxLen            int
	treeMaxDepth          int
	treeMaxLines          int
	treeExcludes          []string
	historyFile           string
}

// WithStdin sets a custom stdin for the readline instance (useful for testing).
func WithStdin(r io.ReadCloser) Option {
	return func(c *shellConfig) {
		c.stdin = r
	}
}

// WithStdout sets a custom stdout writer for streaming output.
func WithStdout(w io.Writer) Option {
	return func(c *shellConfig) {
		c.stdout = w
	}
}

// WithStderr sets a custom stderr writer for banner/error output.
func WithStderr(w io.Writer) Option {
	return func(c *shellConfig) {
		c.stderr = w
	}
}

// WithAgentRunner sets the agent runner for agent-aware operation.
func WithAgentRunner(r *agent.AgentRunner) Option {
	return func(c *shellConfig) {
		c.runner = r
	}
}

// WithCWD sets the working directory for file operations (e.g., Agent 4 code writes).
func WithCWD(path string) Option {
	return func(c *shellConfig) {
		c.cwd = path
	}
}

// WithSandbox sets the sandbox for constrained file operations.
func WithSandbox(sb *sandbox.Sandbox) Option {
	return func(c *shellConfig) {
		c.sandbox = sb
	}
}

// WithStateDir sets the path to the .fakeoid/ state directory for history tracking.
func WithStateDir(path string) Option {
	return func(c *shellConfig) {
		c.stateDir = path
	}
}

// WithHistoryLimit sets the readline history limit.
func WithHistoryLimit(n int) Option {
	return func(c *shellConfig) { c.historyLimit = n }
}

// WithStartupHistoryMax sets the maximum history records shown at startup.
func WithStartupHistoryMax(n int) Option {
	return func(c *shellConfig) { c.startupHistoryMax = n }
}

// WithHistoryTrimPct sets the context usage percentage that triggers history trimming.
func WithHistoryTrimPct(n int) Option {
	return func(c *shellConfig) { c.historyTrimPct = n }
}

// WithTerminalWidthFallback sets the fallback terminal width.
func WithTerminalWidthFallback(n int) Option {
	return func(c *shellConfig) { c.terminalWidthFallback = n }
}

// WithTaskNameTruncLen sets the task name truncation length in history display.
func WithTaskNameTruncLen(n int) Option {
	return func(c *shellConfig) { c.taskNameTruncLen = n }
}

// WithTokenBudgetPct sets the token budget percentage for file injection.
func WithTokenBudgetPct(n int) Option {
	return func(c *shellConfig) { c.tokenBudgetPct = n }
}

// WithTokenCharDivisor sets the character-to-token divisor.
func WithTokenCharDivisor(n int) Option {
	return func(c *shellConfig) { c.tokenCharDivisor = n }
}

// WithMaxIterations sets the maximum feedback loop iterations.
func WithMaxIterations(n int) Option {
	return func(c *shellConfig) { c.maxIterations = n }
}

// WithBuildVerification enables or disables Agent 4 build verification before handoff.
func WithBuildVerification(enabled bool) Option {
	return func(c *shellConfig) { c.buildVerification = &enabled }
}

// WithSlugMaxLen sets the maximum slug length for task filenames.
func WithSlugMaxLen(n int) Option {
	return func(c *shellConfig) { c.slugMaxLen = n }
}

// WithTreeMaxDepth sets the file tree scan max depth.
func WithTreeMaxDepth(n int) Option {
	return func(c *shellConfig) { c.treeMaxDepth = n }
}

// WithTreeMaxLines sets the file tree output max lines.
func WithTreeMaxLines(n int) Option {
	return func(c *shellConfig) { c.treeMaxLines = n }
}

// WithTreeExcludes sets the file tree excluded directory names.
func WithTreeExcludes(excludes []string) Option {
	return func(c *shellConfig) { c.treeExcludes = excludes }
}

// WithHistoryFile sets the history index filename.
func WithHistoryFile(name string) Option {
	return func(c *shellConfig) { c.historyFile = name }
}

// New creates a new Shell instance with readline configured for interactive input.
func New(client ChatClient, ctxSize int, modelName, gpuName, historyPath string, opts ...Option) (*Shell, error) {
	cfg := &shellConfig{
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	histLimit := cfg.historyLimit
	if histLimit == 0 {
		histLimit = 500
	}

	rlCfg := &readline.Config{
		Prompt:          ColorPrompt.Sprint("fakeoid> "),
		HistoryFile:     historyPath,
		HistoryLimit:    histLimit,
		InterruptPrompt: "^C",
		Stdout:          cfg.stdout,
		Stderr:          cfg.stderr,
	}

	if cfg.stdin != nil {
		rlCfg.Stdin = cfg.stdin
	}

	rl, err := readline.NewEx(rlCfg)
	if err != nil {
		return nil, fmt.Errorf("initialize readline: %w", err)
	}

	// Apply defaults for configurable values
	startupMax := cfg.startupHistoryMax
	if startupMax == 0 {
		startupMax = 5
	}
	trimPct := cfg.historyTrimPct
	if trimPct == 0 {
		trimPct = 80
	}
	termWidth := cfg.terminalWidthFallback
	if termWidth == 0 {
		termWidth = 80
	}
	truncLen := cfg.taskNameTruncLen
	if truncLen == 0 {
		truncLen = 40
	}
	budgetPct := cfg.tokenBudgetPct
	if budgetPct == 0 {
		budgetPct = 60
	}
	charDiv := cfg.tokenCharDivisor
	if charDiv == 0 {
		charDiv = 4
	}
	maxIter := cfg.maxIterations
	if maxIter == 0 {
		maxIter = 10
	}
	buildVerify := true
	if cfg.buildVerification != nil {
		buildVerify = *cfg.buildVerification
	}
	slugMax := cfg.slugMaxLen
	if slugMax == 0 {
		slugMax = 50
	}
	treeDepth := cfg.treeMaxDepth
	if treeDepth == 0 {
		treeDepth = 3
	}
	treeLines := cfg.treeMaxLines
	if treeLines == 0 {
		treeLines = 200
	}
	histFile := cfg.historyFile
	if histFile == "" {
		histFile = "history.json"
	}

	return &Shell{
		rl:                    rl,
		client:                client,
		runner:                cfg.runner,
		cwd:                   cfg.cwd,
		sandbox:               cfg.sandbox,
		stateDir:              cfg.stateDir,
		ctxSize:               ctxSize,
		modelName:             modelName,
		gpuName:               gpuName,
		stdout:                cfg.stdout,
		stderr:                cfg.stderr,
		historyLimit:          histLimit,
		startupHistoryMax:     startupMax,
		historyTrimPct:        trimPct,
		terminalWidthFallback: termWidth,
		taskNameTruncLen:      truncLen,
		tokenBudgetPct:        budgetPct,
		tokenCharDivisor:      charDiv,
		maxIterations:         maxIter,
		buildVerification:     buildVerify,
		slugMaxLen:            slugMax,
		treeMaxDepth:          treeDepth,
		treeMaxLines:          treeLines,
		treeExcludes:          cfg.treeExcludes,
		historyFile:           histFile,
	}, nil
}

// Close cleans up the readline instance.
func (s *Shell) Close() {
	if s.rl != nil {
		s.rl.Close()
	}
}

// readMultiLine reads input from readline. Single enter submits immediately.
// To enter multi-line input, end a line with backslash (\) to continue on the
// next line. Returns the joined lines with \n separators.
func (s *Shell) readMultiLine() (string, error) {
	line, err := s.rl.Readline()
	if err != nil {
		return "", err
	}

	first := strings.TrimSpace(line)
	if first == "" {
		return "", nil
	}

	// Single-line input: submit immediately unless line ends with backslash
	if !strings.HasSuffix(first, "\\") {
		return first, nil
	}

	// Multi-line continuation: strip trailing backslash and keep reading
	lines := []string{strings.TrimSuffix(first, "\\")}

	origPrompt := ColorPrompt.Sprint("fakeoid> ")
	s.rl.SetPrompt("  ... > ")

	for {
		line, err = s.rl.Readline()
		if err == io.EOF {
			s.lastReadEOF = true
			break
		}
		if err != nil {
			s.rl.SetPrompt(origPrompt)
			return "", err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		if strings.HasSuffix(trimmed, "\\") {
			lines = append(lines, strings.TrimSuffix(trimmed, "\\"))
			continue
		}
		lines = append(lines, trimmed)
		break
	}

	s.rl.SetPrompt(origPrompt)

	return strings.Join(lines, "\n"), nil
}

// readPasteAware reads input with bracketed paste mode enabled.
// When the terminal wraps pasted text in ESC[200~...ESC[201~, embedded newlines
// are accumulated rather than triggering submission. Only a bare keyboard Enter
// (outside a paste bracket) submits the input.
//
// Falls back to s.rl.Readline() if stdin is not a TTY (tests, pipes).
func (s *Shell) readPasteAware() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return s.rl.Readline()
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return s.rl.Readline()
	}
	defer term.Restore(fd, oldState)

	// Enable bracketed paste mode; disable on return.
	fmt.Fprint(os.Stdout, "\x1b[?2004h")
	defer fmt.Fprint(os.Stdout, "\x1b[?2004l")

	// Print the prompt manually (readline is not driving output in raw mode).
	fmt.Fprint(os.Stdout, ColorPrompt.Sprint("fakeoid> "))

	var buf strings.Builder
	var inPaste bool
	reader := bufio.NewReader(os.Stdin)

	for {
		b, readErr := reader.ReadByte()
		if readErr != nil {
			return buf.String(), readErr
		}

		switch {
		case b == 0x03: // Ctrl+C
			fmt.Fprint(os.Stdout, "\r\n")
			return "", readline.ErrInterrupt

		case b == 0x04: // Ctrl+D / EOF
			fmt.Fprint(os.Stdout, "\r\n")
			if buf.Len() == 0 {
				return "", io.EOF
			}
			return strings.TrimSpace(buf.String()), nil

		case b == 0x7f || b == 0x08: // Backspace / DEL
			str := buf.String()
			if len(str) > 0 {
				runes := []rune(str)
				buf.Reset()
				buf.WriteString(string(runes[:len(runes)-1]))
				fmt.Fprint(os.Stdout, "\b \b")
			}

		case b == 0x1b: // ESC — peek for bracketed paste sequences
			seq := s.readEscapeSeq(reader)
			switch seq {
			case "[200~":
				inPaste = true
			case "[201~":
				inPaste = false
			}

		case b == '\r' || b == '\n':
			if inPaste {
				// Pasted newline — accumulate, do not submit
				buf.WriteByte('\n')
				fmt.Fprint(os.Stdout, "\r\n")
			} else {
				// Keyboard Enter — submit
				fmt.Fprint(os.Stdout, "\r\n")
				return strings.TrimSpace(buf.String()), nil
			}

		default:
			buf.WriteByte(b)
			fmt.Fprintf(os.Stdout, "%c", rune(b))
		}
	}
}

// readEscapeSeq reads an ANSI escape sequence from r after the leading ESC byte
// has already been consumed. Reads until a letter or '~' terminates the sequence.
// Returns the sequence without the leading ESC (e.g. "[200~" for bracketed paste start).
func (s *Shell) readEscapeSeq(r *bufio.Reader) string {
	var seq strings.Builder
	for {
		b, err := r.ReadByte()
		if err != nil {
			break
		}
		seq.WriteByte(b)
		// Sequences end at a letter (A-Z, a-z) or '~'
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~' {
			break
		}
	}
	return seq.String()
}

// getHistory returns the conversation history, using the runner if available.
func (s *Shell) getHistory() []server.Message {
	if s.runner != nil {
		return s.runner.History()
	}
	return s.history
}

// appendUserMessage adds a user message to the appropriate history store.
func (s *Shell) appendUserMessage(content string) {
	if s.runner != nil {
		s.runner.AppendUserMessage(content)
	} else {
		s.history = append(s.history, server.Message{
			Role:    "user",
			Content: content,
		})
	}
}

// appendAssistantMessage adds an assistant message to the appropriate history store.
func (s *Shell) appendAssistantMessage(content string) {
	if s.runner != nil {
		s.runner.AppendAssistantMessage(content)
	} else {
		s.history = append(s.history, server.Message{
			Role:    "assistant",
			Content: content,
		})
	}
}

// removeLastUserMessage removes the most recent user message (for cancellation/error).
func (s *Shell) removeLastUserMessage() {
	if s.runner != nil {
		// Runner doesn't expose remove, but we can track this via history length.
		// For now, the runner's internal history doesn't support removal.
		// We'll need to handle this differently -- skip removal for runner mode
		// since the AgentRunner doesn't expose slice manipulation.
		// Actually, let's not remove -- the user message is valid, just the response was cancelled.
		return
	}
	if len(s.history) > 0 {
		s.history = s.history[:len(s.history)-1]
	}
}

// Run starts the interactive REPL loop. It returns nil on clean exit (Ctrl+C at prompt or EOF).
func (s *Shell) Run(ctx context.Context) error {
	PrintBanner(s.stderr, s.modelName, s.gpuName)
	PrintWelcome(s.stderr)

	if s.stateDir != "" {
		s.showStartupHistory()
	}

	for {
		// Check if parent context is cancelled
		if ctx.Err() != nil {
			return nil
		}

		var input string
		var err error
		if s.runner != nil {
			if a1, ok := s.runner.Active().(*agent.Agent1); ok && a1.IsGathering() {
				input, err = s.readPasteAware()
			} else {
				input, err = s.readMultiLine()
			}
		} else {
			input, err = s.readMultiLine()
		}
		if err == readline.ErrInterrupt {
			return nil
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("readline error: %w", err)
		}
		if input == "" {
			if s.lastReadEOF {
				return nil
			}
			continue
		}
		// Process input, then check if EOF was reached during multi-line read
		exitAfter := s.lastReadEOF

		// Shell command: resume
		if strings.ToLower(strings.TrimSpace(input)) == "resume" {
			if s.stateDir == "" {
				fmt.Fprintln(s.stderr, "No state directory configured.")
			} else {
				if err := s.resumeLastTask(ctx); err != nil {
					ColorError.Fprintf(s.stderr, "Resume error: %s\n", err)
				}
				// Reactivate Agent 1 after resume
				if s.runner != nil {
					a1 := agent.NewAgent1(s.cwd, s.runner.TaskDir())
					s.runner.SwitchAgent(a1)
				}
			}
			if exitAfter {
				return nil
			}
			continue
		}

		// Shell commands: /history N
		if strings.HasPrefix(input, "/history ") {
			numStr := strings.TrimPrefix(input, "/history ")
			num, parseErr := strconv.Atoi(strings.TrimSpace(numStr))
			if parseErr != nil {
				fmt.Fprintln(s.stderr, "Usage: /history <task-number>")
			} else {
				s.historyDetail(num)
			}
			if exitAfter {
				return nil
			}
			continue
		}

		// Agent-aware trigger detection
		if s.runner != nil {
			if a1, ok := s.runner.Active().(*agent.Agent1); ok && a1.IsTrigger(input) {
				action := a1.ProcessTrigger()

				if action.Type == agent.ActionGenerate {
					// Append a generation instruction as user message
					s.runner.AppendUserMessage("Generate the task prompt now.")

					// Stream LLM response to get the task prompt
					responseBuf, streamErr := s.streamResponse(ctx)
					if streamErr != nil {
						ColorError.Fprintf(s.stderr, "Error: %s\n", streamErr)
						if exitAfter {
							return nil
						}
						continue
					}

					// Write the response as a task file
					path, writeErr := agent.WriteTaskFile(s.sandbox, a1.TaskDir(), responseBuf)
					if writeErr != nil {
						ColorError.Fprintf(s.stderr, "Error writing task file: %s\n", writeErr)
						if exitAfter {
							return nil
						}
						continue
					}

					// Append the LLM's response as assistant message
					s.runner.AppendAssistantMessage(responseBuf)

					fmt.Fprintf(s.stderr, "Task prompt written to %s\n", path)
					a1.SetConfirming()
					fmt.Fprintln(s.stderr, "Say 'go' to hand off, or add corrections.")

					if exitAfter {
						return nil
					}
					continue
				}

				if action.Type == agent.ActionComplete {
					// Get the task file path from the task directory
					taskFilePath := getLastTaskFile(s.runner.TaskDir())
					if err := s.runPipeline(ctx, taskFilePath); err != nil {
						ColorError.Fprintf(s.stderr, "Pipeline error: %s\n", err)
					}
					// Reactivate Agent 1 and continue REPL
					a1 := agent.NewAgent1(s.cwd, s.runner.TaskDir())
					s.runner.SwitchAgent(a1)
					if exitAfter {
						return nil
					}
					continue
				}
			} else if s.runner.Active() != nil {
				// Not a trigger -- check if we need to reset confirming state
				if a1, ok := s.runner.Active().(*agent.Agent1); ok && a1.IsConfirming() {
					// User is providing corrections instead of confirming
					a1.SetGathering()
				}
			}
		}

		// Normal conversation flow
		s.appendUserMessage(input)

		// Print agent tag
		if s.runner != nil && s.runner.Active() != nil {
			PrintAgentTag(s.stderr, s.runner.Active().Number(), s.runner.Active().Name())
		} else {
			PrintAgentTag(s.stderr, 1, "")
		}

		// Two-tier Ctrl+C: create a cancellable stream context
		streamCtx, streamCancel := context.WithCancel(ctx)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT)
		go func() {
			select {
			case <-sigCh:
				streamCancel()
			case <-streamCtx.Done():
			}
		}()

		// Create display writer for word-wrapped streaming output
		dw := NewDisplayWriter(s.stdout, GetTermWidthWithFallback(s.terminalWidthFallback))

		// Capture response text alongside display
		var responseBuf strings.Builder
		onToken := func(token string) {
			dw.WriteToken(token)
			responseBuf.WriteString(token)
		}

		// Stream the LLM response
		result, streamErr := s.client.StreamChatCompletion(streamCtx, s.getHistory(), onToken)
		dw.Flush()
		fmt.Fprintln(s.stdout)

		// Capture cancellation state before cleanup
		wasCancelled := streamCtx.Err() != nil

		// Clean up signal handler
		signal.Stop(sigCh)
		streamCancel()

		// Handle stream result
		if wasCancelled {
			// Stream was cancelled (Ctrl+C during streaming)
			s.removeLastUserMessage()
			fmt.Fprintln(s.stderr)
			if exitAfter {
				return nil
			}
			continue
		}

		if streamErr != nil {
			// Non-cancellation error: print error, remove user message
			ColorError.Fprintf(s.stderr, "Error: %s\n", streamErr)
			s.removeLastUserMessage()
			if exitAfter {
				return nil
			}
			continue
		}

		// Success: append assistant message to history
		s.appendAssistantMessage(responseBuf.String())
		s.usedTokens = result.Usage.TotalTokens

		// Let active agent process the response
		if s.runner != nil && s.runner.Active() != nil {
			action := s.runner.Active().HandleResponse(responseBuf.String())

			// If Agent 1 detected a complete task prompt, auto-proceed
			if a1, ok := s.runner.Active().(*agent.Agent1); ok && action.Type == agent.ActionComplete {
				// Write the response as a task file
				path, writeErr := agent.WriteTaskFile(s.sandbox, a1.TaskDir(), responseBuf.String())
				if writeErr != nil {
					ColorError.Fprintf(s.stderr, "Error writing task file: %s\n", writeErr)
				} else {
					fmt.Fprintf(s.stderr, "\nTask prompt written to %s\n", path)
					// Run the full pipeline
					if err := s.runPipeline(ctx, path); err != nil {
						ColorError.Fprintf(s.stderr, "Pipeline error: %s\n", err)
					}
					// Reactivate Agent 1 for next task
					newA1 := agent.NewAgent1(s.cwd, s.runner.TaskDir())
					s.runner.SwitchAgent(newA1)
				}
				if exitAfter {
					return nil
				}
				continue
			}
		}

		s.trimHistory()

		if exitAfter {
			return nil
		}
	}
}

// streamResponse streams an LLM response, printing tokens as they arrive.
// Returns the full response text or an error.
func (s *Shell) streamResponse(ctx context.Context) (string, error) {
	streamCtx, streamCancel := context.WithCancel(ctx)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		select {
		case <-sigCh:
			streamCancel()
		case <-streamCtx.Done():
		}
	}()

	dw := NewDisplayWriter(s.stdout, GetTermWidthWithFallback(s.terminalWidthFallback))
	var responseBuf strings.Builder
	onToken := func(token string) {
		dw.WriteToken(token)
		responseBuf.WriteString(token)
	}

	// Print agent tag before streaming
	if s.runner != nil && s.runner.Active() != nil {
		PrintAgentTag(s.stderr, s.runner.Active().Number(), s.runner.Active().Name())
	}

	result, streamErr := s.client.StreamChatCompletion(streamCtx, s.getHistory(), onToken)
	dw.Flush()
	fmt.Fprintln(s.stdout)

	// Capture cancellation state before cleanup
	wasCancelled := streamCtx.Err() != nil

	signal.Stop(sigCh)
	streamCancel()

	if wasCancelled {
		return "", fmt.Errorf("stream cancelled")
	}
	if streamErr != nil {
		return "", streamErr
	}

	s.usedTokens = result.Usage.TotalTokens
	return responseBuf.String(), nil
}

// showStartupHistory loads and displays task history on shell startup.
// Returns true if the last task is resumable (interrupted or failed).
func (s *Shell) showStartupHistory() bool {
	idx, _ := state.LoadHistory(filepath.Join(s.stateDir, s.historyFile))
	if len(idx.Records) == 0 {
		PrintFirstRun(s.stderr)
		fmt.Fprintln(s.stderr)
		return false
	}
	records := idx.Records
	if len(records) > s.startupHistoryMax {
		records = records[len(records)-s.startupHistoryMax:]
	}
	PrintHistoryTable(s.stderr, records, s.taskNameTruncLen)
	fmt.Fprintln(s.stderr)

	last := idx.Records[len(idx.Records)-1]
	if last.Outcome == "interrupted" || last.Outcome == "failure" {
		taskFile := filepath.Join(s.stateDir, last.TaskFile)
		stage := detectResumeStage(filepath.Dir(taskFile), strings.TrimSuffix(filepath.Base(taskFile), ".md"))
		fmt.Fprintf(s.stderr, "Last task \"%s\" was %s (will resume at %s).\n", last.TaskName, last.Outcome, stage)
		fmt.Fprintln(s.stderr, "Type 'resume' to continue, or start a new task.")
		fmt.Fprintln(s.stderr)
		return true
	}
	return false
}

// recordOutcome writes frontmatter to the task file and appends a history record.
// Errors are logged to stderr but do not fail the pipeline.
func (s *Shell) recordOutcome(sessionID, taskFilePath, outcome, testResult string, startTime time.Time, filesModified []string, agents []state.AgentOutcome) {
	if s.stateDir == "" || s.sandbox == nil {
		return
	}

	duration := time.Since(startTime).Seconds()
	fm := state.TaskFrontmatter{
		Timestamp:     startTime,
		SessionID:     sessionID,
		Outcome:       outcome,
		Agents:        agents,
		DurationSec:   duration,
		FilesModified: filesModified,
		TestResult:    testResult,
	}

	// Enrich task file with frontmatter
	content, err := os.ReadFile(taskFilePath)
	if err == nil {
		enriched, fmErr := state.InjectFrontmatter(fm, string(content))
		if fmErr == nil {
			relPath, relErr := filepath.Rel(s.sandbox.CWD(), taskFilePath)
			if relErr == nil {
				if wErr := s.sandbox.WriteFile(relPath, []byte(enriched), 0o644); wErr != nil {
					fmt.Fprintf(s.stderr, "Warning: could not write frontmatter: %s\n", wErr)
				}
			}
		}
	}

	// Append history record
	taskName := strings.TrimSuffix(filepath.Base(taskFilePath), ".md")
	record := state.HistoryRecord{
		SessionID: sessionID,
		Timestamp: startTime,
		TaskName:  taskName,
		Outcome:   outcome,
		TaskFile:  filepath.Join("tasks", filepath.Base(taskFilePath)),
	}
	if err := state.AppendRecord(s.sandbox, s.stateDir, record); err != nil {
		fmt.Fprintf(s.stderr, "Warning: could not append history record: %s\n", err)
	}
}

// historyDetail displays the per-agent outcome detail for a specific task by index.
func (s *Shell) historyDetail(index int) {
	if s.stateDir == "" {
		fmt.Fprintln(s.stderr, "No history available (state directory not configured)")
		return
	}
	histPath := filepath.Join(s.stateDir, "history.json")
	idx, err := state.LoadHistory(histPath)
	if err != nil || len(idx.Records) == 0 {
		fmt.Fprintln(s.stderr, "No history records found")
		return
	}
	if index < 1 || index > len(idx.Records) {
		fmt.Fprintf(s.stderr, "Invalid task number: %d (valid range: 1-%d)\n", index, len(idx.Records))
		return
	}
	record := idx.Records[index-1]

	// Read the task file to get frontmatter with AgentOutcome data
	taskFilePath := filepath.Join(s.stateDir, record.TaskFile)
	content, err := os.ReadFile(taskFilePath)
	if err != nil {
		fmt.Fprintf(s.stderr, "Could not read task file %s: %v\n", record.TaskFile, err)
		return
	}
	fm, _, err := state.ParseFrontmatter(string(content))
	if err != nil {
		fmt.Fprintf(s.stderr, "Could not parse task frontmatter: %v\n", err)
		return
	}
	PrintHistoryDetail(s.stderr, record.TaskName, fm)
}

// trimHistory drops oldest user+assistant pairs when context usage exceeds the configured threshold.
func (s *Shell) trimHistory() {
	threshold := s.ctxSize * s.historyTrimPct / 100

	if s.runner != nil {
		for s.usedTokens > threshold && s.runner.HistoryLen() > 3 {
			dropped := s.runner.TrimOldest()
			s.usedTokens -= dropped
			if s.usedTokens < 0 {
				s.usedTokens = 0
			}
		}
		return
	}

	// Fallback: direct history management
	for s.usedTokens > threshold && len(s.history) > 2 {
		// Estimate tokens for the two oldest messages
		dropped := len(s.history[0].Content)/4 + len(s.history[1].Content)/4
		s.history = s.history[2:]
		s.usedTokens -= dropped
		if s.usedTokens < 0 {
			s.usedTokens = 0
		}
	}
}

// runAgent2Phase drives Agent 2 (Prompt Engineer) through a multi-turn read loop.
// It creates Agent 2, injects the task file content, streams LLM responses, fulfills
// file read requests via sandbox, and writes the enriched prompt file.
// Returns the path to the enriched file, or error.
func (s *Shell) runAgent2Phase(ctx context.Context, taskFilePath string) (string, error) {
	a2Start := time.Now()
	cwd := s.cwd
	if s.sandbox != nil {
		cwd = s.sandbox.CWD()
	}

	a2 := agent.NewAgent2(cwd, s.runner.TaskDir(), taskFilePath, s.ctxSize)
	s.runner.SwitchAgent(a2)

	// Read task file and inject as first user message
	taskContent, err := os.ReadFile(taskFilePath)
	if err != nil {
		return "", fmt.Errorf("read task file: %w", err)
	}
	s.runner.AppendUserMessage("Here is the task prompt to enrich:\n\n" + string(taskContent))

	// Multi-turn loop
	var lastResponse string
	for {
		responseBuf, streamErr := s.streamResponse(ctx)
		if streamErr != nil {
			return "", streamErr
		}
		s.runner.AppendAssistantMessage(responseBuf)
		lastResponse = responseBuf

		action := a2.HandleResponse(responseBuf)
		if action.Type == agent.ActionComplete {
			break
		}

		// Parse read requests and fulfill them via sandbox
		readPaths := agent.ParseReadBlocks(responseBuf)
		var injection strings.Builder
		for _, p := range readPaths {
			absPath := filepath.Join(cwd, p)
			if err := s.sandbox.ValidateRead(absPath); err != nil {
				fmt.Fprintf(&injection, "File %s: blocked by sandbox (%v)\n\n", p, err)
				continue
			}
			data, readErr := os.ReadFile(absPath)
			if readErr != nil {
				fmt.Fprintf(&injection, "File %s: read error: %s\n\n", p, readErr)
				continue
			}
			// Track in agent for token budget
			a2.AddReadFile(p, string(data))
			// Determine language from extension for fenced block annotation
			ext := filepath.Ext(p)
			lang := "text"
			if ext == ".go" {
				lang = "go"
			} else if ext == ".md" {
				lang = "markdown"
			} else if ext == ".json" {
				lang = "json"
			} else if ext == ".yaml" || ext == ".yml" {
				lang = "yaml"
			}
			fmt.Fprintf(&injection, "```%s:%s\n%s\n```\n\n", lang, p, string(data))
		}
		if injection.Len() > 0 {
			s.runner.AppendUserMessage(injection.String())
		}
	}

	// Write enriched file
	enrichedPath, err := agent.WriteEnrichedFile(
		s.sandbox, s.runner.TaskDir(), filepath.Base(taskFilePath), lastResponse)
	if err != nil {
		return "", fmt.Errorf("write enriched file: %w", err)
	}

	fmt.Fprintf(s.stderr, "Enriched prompt written to %s\n", enrichedPath)

	// Persist Agent 2 conversation (non-fatal)
	if s.sandbox != nil {
		taskBase := filepath.Base(taskFilePath)
		_, convErr := agent.WriteConversationFile(s.sandbox, s.runner.TaskDir(), taskBase, 2, "Prompt Engineer", 0, s.runner.History(), time.Since(a2Start))
		if convErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write Agent 2 conversation: %v\n", convErr)
		}
	}

	return enrichedPath, nil
}

// runAgent3Phase drives Agent 3 (Software Architect) through a single-turn LLM call.
// It runs AST extraction, creates Agent 3, streams one response, and writes the
// change plan file. Returns the path to the change plan file, or error.
func (s *Shell) runAgent3Phase(ctx context.Context, taskFilePath string) (string, error) {
	a3Start := time.Now()
	cwd := s.cwd
	if s.sandbox != nil {
		cwd = s.sandbox.CWD()
	} else if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Run AST extraction (non-fatal if it fails)
	astMarkdown := ""
	ext := extract.NewGoExtractor()
	info, err := ext.Extract(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: AST extraction failed: %v\n", err)
	} else {
		astMarkdown = extract.RenderMarkdown(info)
	}

	a3 := agent.NewAgent3(cwd, s.runner.TaskDir(), taskFilePath, astMarkdown)
	s.runner.SwitchAgent(a3)

	// Read task file content and inject as user message
	taskContent, err := os.ReadFile(taskFilePath)
	if err != nil {
		return "", fmt.Errorf("read task file: %w", err)
	}
	s.runner.AppendUserMessage("Here is the task prompt:\n\n" + string(taskContent))

	// Single-turn: stream one response
	responseBuf, streamErr := s.streamResponse(ctx)
	if streamErr != nil {
		return "", streamErr
	}

	// HandleResponse for interface consistency (always returns ActionComplete)
	a3.HandleResponse(responseBuf)

	// Write change plan file
	changePlanPath, writeErr := agent.WriteChangePlanFile(s.sandbox, s.runner.TaskDir(), filepath.Base(taskFilePath), responseBuf)
	if writeErr != nil {
		return "", fmt.Errorf("write change plan: %w", writeErr)
	}

	fmt.Fprintf(s.stderr, "Change plan written to %s\n", changePlanPath)

	// Scaffold directories and placeholder files from change plan (non-fatal, per D-13)
	if s.sandbox != nil {
		filePaths := agent.ExtractFilePaths(responseBuf)
		if len(filePaths) > 0 {
			scaffoldErrs := agent.ScaffoldFiles(s.sandbox, filePaths)
			for _, se := range scaffoldErrs {
				fmt.Fprintf(os.Stderr, "Warning: scaffold: %v\n", se)
			}
			if len(filePaths) > len(scaffoldErrs) {
				fmt.Fprintf(s.stderr, "Scaffolded %d file(s) from change plan\n", len(filePaths)-len(scaffoldErrs))
			}
		}
	}

	// Persist Agent 3 conversation (non-fatal)
	if s.sandbox != nil {
		taskBase := filepath.Base(taskFilePath)
		_, convErr := agent.WriteConversationFile(s.sandbox, s.runner.TaskDir(), taskBase, 3, "Software Architect", 0, s.runner.History(), time.Since(a3Start))
		if convErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write Agent 3 conversation: %v\n", convErr)
		}
	}

	return changePlanPath, nil
}

// runPipeline drives the full Agent 4 -> Agent 5 -> Agent 6 -> Agent 7 pipeline with feedback loop.
// After Agent 4 writes code and handoff, it runs the feedback loop.
func (s *Shell) runPipeline(ctx context.Context, taskFilePath string) error {
	startTime := time.Now()
	sessionID := state.GenerateSessionID()
	outcome := (*string)(nil) // nil means "never set" -- defer uses this

	var agentOutcomes []state.AgentOutcome

	// Defer to record interrupted status if outcome was never set
	defer func() {
		if outcome == nil && s.stateDir != "" {
			s.recordOutcome(sessionID, taskFilePath, "interrupted", "", startTime, nil, agentOutcomes)
		}
	}()

	cwd := s.cwd
	if s.sandbox != nil {
		cwd = s.sandbox.CWD()
	} else if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Agent 1: Systems Engineer -- always succeeds (produced the task file)
	fmt.Fprintf(s.stderr, "\n--- Starting Pipeline: Agent 1: Systems Engineer ---\n\n")
	agentOutcomes = append(agentOutcomes, state.AgentOutcome{
		Number: 1, Name: "Systems Engineer", Status: "success",
	})

	// Agent 2: Prompt Engineer -- enrich task prompt with file context
	inputForAgent3 := taskFilePath
	if s.sandbox != nil {
		PrintTransition(s.stderr, 2, "Prompt Engineer")
		enrichedPath, a2Err := s.runAgent2Phase(ctx, taskFilePath)
		if a2Err != nil {
			// Agent 2 failure is non-fatal: Agent 3 gets raw task file
			fmt.Fprintf(os.Stderr, "Warning: Agent 2 failed: %v\n", a2Err)
			agentOutcomes = append(agentOutcomes, state.AgentOutcome{
				Number: 2, Name: "Prompt Engineer", Status: "failed",
			})
		} else {
			inputForAgent3 = enrichedPath
			agentOutcomes = append(agentOutcomes, state.AgentOutcome{
				Number: 2, Name: "Prompt Engineer", Status: "success",
			})
		}
	} else {
		agentOutcomes = append(agentOutcomes, state.AgentOutcome{
			Number: 2, Name: "Prompt Engineer", Status: "skipped",
		})
	}

	// Agent 3: Software Architect -- analyze task and produce change plan
	changePlanPath := ""
	if s.sandbox != nil {
		PrintTransition(s.stderr, 3, "Software Architect")
		var a3Err error
		changePlanPath, a3Err = s.runAgent3Phase(ctx, inputForAgent3)
		if a3Err != nil {
			// Agent 3 failure is non-fatal: Agent 4 proceeds without change plan
			fmt.Fprintf(os.Stderr, "Warning: Agent 3 failed: %v\n", a3Err)
			agentOutcomes = append(agentOutcomes, state.AgentOutcome{
				Number: 3, Name: "Software Architect", Status: "failed",
			})
		} else {
			agentOutcomes = append(agentOutcomes, state.AgentOutcome{
				Number: 3, Name: "Software Architect", Status: "success",
			})
		}
	} else {
		agentOutcomes = append(agentOutcomes, state.AgentOutcome{
			Number: 3, Name: "Software Architect", Status: "skipped",
		})
	}

	PrintTransition(s.stderr, 4, "Software Engineer")
	a4Start := time.Now()
	a4 := agent.NewAgent4(cwd, s.runner.TaskDir(), taskFilePath, changePlanPath)
	s.runner.SwitchAgent(a4)

	// Inject task prompt as first user message
	taskContent, err := os.ReadFile(taskFilePath)
	if err != nil {
		return fmt.Errorf("read task file: %w", err)
	}
	s.runner.AppendUserMessage("Here is the task prompt:\n\n" + string(taskContent))

	// Autonomous loop: stream -> HandleResponse -> check ActionComplete
	var allResponses strings.Builder
	for {
		responseBuf, streamErr := s.streamResponse(ctx)
		if streamErr != nil {
			// Ctrl+C or error: exit without writing files (safe)
			return nil
		}
		s.runner.AppendAssistantMessage(responseBuf)
		allResponses.WriteString(responseBuf)

		action := a4.HandleResponse(responseBuf)
		if action.Type == agent.ActionComplete {
			break
		}
		// Multi-turn: add a continuation message
		s.runner.AppendUserMessage("Continue implementing. Output any remaining code blocks.")
	}

	// Parse code blocks from accumulated responses
	blocks := agent.ParseCodeBlocks(allResponses.String())

	var handoffPath string
	var results []agent.FileResult
	if len(blocks) > 0 {
		var blocked []sandbox.BlockedFile
		results, blocked = agent.WriteCodeBlocks(s.sandbox, blocks)
		// Print file confirmations
		fmt.Fprintln(s.stderr)
		for _, r := range results {
			fmt.Fprintf(s.stderr, "  \u2713 %s (%s)\n", r.Path, r.Action)
		}
		for _, b := range blocked {
			fmt.Fprintf(s.stderr, "  X %s (blocked: %s)\n", b.Path, b.Reason)
		}
		fmt.Fprintln(s.stderr)

		// Verify build before handoff
		var buildErr error
		results, buildErr = s.verifyAgent4Build(ctx, cwd, &allResponses, results)
		if buildErr != nil {
			return buildErr
		}

		// Write handoff file
		taskBase := filepath.Base(taskFilePath)
		hp, handoffErr := agent.WriteHandoffFile(s.sandbox, s.runner.TaskDir(), taskBase, results, allResponses.String())
		if handoffErr != nil {
			ColorError.Fprintf(s.stderr, "Error writing handoff: %s\n", handoffErr)
		} else {
			handoffPath = hp
			fmt.Fprintf(s.stderr, "Handoff written to %s\n", handoffPath)
		}
	} else {
		fmt.Fprintln(s.stderr, "No code blocks produced. Agent 4 may not have used the required format:")
		fmt.Fprintln(s.stderr, "  ```go:path/to/file.go")
		fmt.Fprintln(s.stderr, "  [file content]")
		fmt.Fprintln(s.stderr, "  ```")
	}

	// Agent 4 always runs (core code writer) -- record success
	agentOutcomes = append(agentOutcomes, state.AgentOutcome{
		Number: 4, Name: "Software Engineer", Status: "success",
	})

	// Persist initial Agent 4 conversation (non-fatal, iteration 1)
	if s.sandbox != nil {
		taskBase := filepath.Base(taskFilePath)
		_, convErr := agent.WriteConversationFile(s.sandbox, s.runner.TaskDir(), taskBase, 4, "Software Engineer", 1, s.runner.History(), time.Since(a4Start))
		if convErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write Agent 4 conversation: %v\n", convErr)
		}
	}

	// Collect files modified from results
	var filesModified []string
	for _, r := range results {
		filesModified = append(filesModified, r.Path)
	}

	// Run feedback loop if we have a handoff
	if handoffPath != "" {
		passed, loopOutcomes, err := s.runFeedbackLoop(ctx, handoffPath, taskFilePath)
		agentOutcomes = append(agentOutcomes, loopOutcomes...)
		PrintPipelineSummary(s.stderr, agentOutcomes)
		if err != nil {
			return err
		}
		if passed {
			o := "success"
			outcome = &o
			s.recordOutcome(sessionID, taskFilePath, "success", "pass", startTime, filesModified, agentOutcomes)
		} else {
			o := "failure"
			outcome = &o
			s.recordOutcome(sessionID, taskFilePath, "failure", "fail", startTime, filesModified, agentOutcomes)
		}
	} else {
		// No feedback loop -- Agent 5 skipped
		agentOutcomes = append(agentOutcomes, state.AgentOutcome{
			Number: 5, Name: "QA Team Leader", Status: "skipped",
		})
		PrintPipelineSummary(s.stderr, agentOutcomes)
		o := "success"
		outcome = &o
		s.recordOutcome(sessionID, taskFilePath, "success", "", startTime, filesModified, agentOutcomes)
	}

	return nil
}

// runAgent5Phase drives Agent 5 (QA Team Leader) to analyze the handoff and
// produce a structured test plan that splits work into sub-agents.
// Returns the parsed test plan entries and any error.
func (s *Shell) runAgent5Phase(ctx context.Context, handoffPath string, taskFileName string, iteration int) ([]agent.TestPlanEntry, error) {
	a5Start := time.Now()
	cwd := s.cwd
	if s.sandbox != nil {
		cwd = s.sandbox.CWD()
	} else if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Read README.md for build instructions (used by Agent 5 prompt)
	readmeContent := agent.ReadBuildInstructions(cwd, s.sandbox)

	// Read task requirements: prefer enriched prompt, fall back to raw task file
	taskRequirements := ""
	base := strings.TrimSuffix(taskFileName, ".md")
	enrichedPath := filepath.Join(s.runner.TaskDir(), base+"-enriched.md")
	taskPath := filepath.Join(s.runner.TaskDir(), taskFileName)
	if data, err := os.ReadFile(enrichedPath); err == nil {
		taskRequirements = string(data)
	} else if data, err := os.ReadFile(taskPath); err == nil {
		taskRequirements = string(data)
	}

	a5 := agent.NewAgent5(cwd, s.runner.TaskDir(), handoffPath, s.sandbox, readmeContent, taskRequirements)
	s.runner.SwitchAgent(a5)

	// Inject instruction as first user message
	s.runner.AppendUserMessage("Analyze the code changes from the handoff and produce a structured test plan. Split testing into blackbox (sanity/smoke) and whitebox (unit/integration) scopes.")

	// Autonomous loop: stream -> HandleResponse -> break on ActionComplete
	var allResponses strings.Builder
	for {
		responseBuf, streamErr := s.streamResponse(ctx)
		if streamErr != nil {
			return nil, streamErr
		}
		s.runner.AppendAssistantMessage(responseBuf)
		allResponses.WriteString(responseBuf)

		action := a5.HandleResponse(responseBuf)
		if action.Type == agent.ActionComplete {
			break
		}
		s.runner.AppendUserMessage("Continue producing the test plan. End with ## END TEST PLAN.")
	}

	// Persist Agent 5 conversation (non-fatal)
	if s.sandbox != nil {
		_, convErr := agent.WriteConversationFile(s.sandbox, s.runner.TaskDir(), taskFileName, 5, "QA Team Leader", iteration, s.runner.History(), time.Since(a5Start))
		if convErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write Agent 5 conversation: %v\n", convErr)
		}
	}

	// Parse the test plan
	entries := agent.ParseTestPlan(allResponses.String())
	if len(entries) == 0 {
		return nil, fmt.Errorf("Agent 5 produced no test plan entries")
	}

	return entries, nil
}

// runAgent6SubPhase drives a single Agent 6 (QA Tester) sub-agent instance.
// Returns the test output, whether tests passed, and any error.
func (s *Shell) runAgent6SubPhase(ctx context.Context, a6 *agent.Agent6, handoffPath string, taskFileName string, iteration int) (string, bool, error) {
	a6Start := time.Now()
	cwd := s.cwd
	if s.sandbox != nil {
		cwd = s.sandbox.CWD()
	} else if cwd == "" {
		cwd, _ = os.Getwd()
	}

	readmeContent := agent.ReadBuildInstructions(cwd, s.sandbox)
	s.runner.SwitchAgent(a6)

	// Inject instruction based on scope
	if a6.Scope() == agent.ScopeBlackBox {
		s.runner.AppendUserMessage("Write and output the test.sh script for sanity and smoke testing. Follow the instructions in your system prompt.")
	} else {
		s.runner.AppendUserMessage("Write unit/integration tests for the source code. Follow the instructions in your system prompt.")
	}

	// Autonomous loop
	var allResponses strings.Builder
	for {
		responseBuf, streamErr := s.streamResponse(ctx)
		if streamErr != nil {
			return "", false, streamErr
		}
		s.runner.AppendAssistantMessage(responseBuf)
		allResponses.WriteString(responseBuf)

		action := a6.HandleResponse(responseBuf)
		if action.Type == agent.ActionComplete {
			break
		}
		s.runner.AppendUserMessage("Continue writing tests. Output any remaining code blocks.")
	}

	// Parse and write test files
	blocks := agent.ParseCodeBlocks(allResponses.String())
	if len(blocks) > 0 {
		results, blocked := agent.WriteCodeBlocks(s.sandbox, blocks)
		fmt.Fprintln(s.stderr)
		for _, r := range results {
			fmt.Fprintf(s.stderr, "  \u2713 %s (%s)\n", r.Path, r.Action)
		}
		for _, b := range blocked {
			fmt.Fprintf(s.stderr, "  X %s (blocked: %s)\n", b.Path, b.Reason)
		}
		fmt.Fprintln(s.stderr)
	}

	// Run tests based on scope
	var testOutput string
	var passed bool
	var testErr error

	isGo := agent.IsGoProject(cwd)

	if a6.Scope() == agent.ScopeBlackBox {
		// Black-box: run test.sh or README build commands
		testOutput, passed, testErr = agent.RunTestScript(ctx, cwd, readmeContent, s.stderr)
	} else {
		// White-box: run language-specific tests
		if isGo {
			handoffContent, err := os.ReadFile(handoffPath)
			if err != nil {
				return "", false, fmt.Errorf("read handoff: %w", err)
			}
			packages := agent.ExtractPackages(string(handoffContent))
			if len(packages) == 0 {
				packages = []string{"./..."}
			}
			testOutput, passed, testErr = agent.RunGoTest(ctx, cwd, packages, s.stderr)
		} else {
			testOutput, passed, testErr = agent.RunTestScript(ctx, cwd, readmeContent, s.stderr)
		}
	}
	if testErr != nil {
		return testOutput, false, testErr
	}

	// Check for compilation error in whitebox -- retry once
	if !passed && a6.Scope() == agent.ScopeWhiteBox && isCompilationError(testOutput) {
		s.runner.AppendUserMessage("The tests have a compilation error. Fix the test code:\n\n```\n" + testOutput + "\n```")
		var retryResponses strings.Builder
		for {
			responseBuf, streamErr := s.streamResponse(ctx)
			if streamErr != nil {
				return testOutput, false, streamErr
			}
			s.runner.AppendAssistantMessage(responseBuf)
			retryResponses.WriteString(responseBuf)

			action := a6.HandleResponse(responseBuf)
			if action.Type == agent.ActionComplete {
				break
			}
		}

		retryBlocks := agent.ParseCodeBlocks(retryResponses.String())
		if len(retryBlocks) > 0 {
			results, blocked := agent.WriteCodeBlocks(s.sandbox, retryBlocks)
			for _, r := range results {
				fmt.Fprintf(s.stderr, "  \u2713 %s (%s)\n", r.Path, r.Action)
			}
			for _, b := range blocked {
				fmt.Fprintf(s.stderr, "  X %s (blocked: %s)\n", b.Path, b.Reason)
			}
		}

		if isGo {
			handoffContent2, _ := os.ReadFile(handoffPath)
			packages2 := agent.ExtractPackages(string(handoffContent2))
			if len(packages2) == 0 {
				packages2 = []string{"./..."}
			}
			testOutput, passed, testErr = agent.RunGoTest(ctx, cwd, packages2, s.stderr)
		} else {
			testOutput, passed, testErr = agent.RunTestScript(ctx, cwd, readmeContent, s.stderr)
		}
		if testErr != nil {
			return testOutput, false, testErr
		}
	}

	// Persist Agent 6 sub-agent conversation (non-fatal)
	scopeLabel := "QA Tester (Black-Box)"
	if a6.Scope() == agent.ScopeWhiteBox {
		scopeLabel = "QA Tester (White-Box)"
	}
	if s.sandbox != nil {
		_, convErr := agent.WriteConversationFile(s.sandbox, s.runner.TaskDir(), taskFileName, 6, scopeLabel, iteration, s.runner.History(), time.Since(a6Start))
		if convErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write Agent 6 conversation: %v\n", convErr)
		}
	}

	// Write test results file (non-fatal)
	if s.sandbox != nil {
		testFileCount := len(blocks)
		var testFilePaths []string
		for _, b := range blocks {
			testFilePaths = append(testFilePaths, b.FilePath)
		}
		_, trErr := agent.WriteTestResultsFile(s.sandbox, s.runner.TaskDir(), taskFileName, iteration, passed, testFileCount, testFilePaths)
		if trErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write test results: %v\n", trErr)
		}
	}

	return testOutput, passed, nil
}

// runAgent6TestingPhase orchestrates the full Agent 6 testing pipeline:
// Agent 6.1 (blackbox) runs first; if it passes, Agent 6.2 (whitebox) runs.
// Returns the combined test output, whether all tests passed, and any error.
func (s *Shell) runAgent6TestingPhase(ctx context.Context, testPlan []agent.TestPlanEntry, handoffPath string, taskFileName string, iteration int) (string, bool, error) {
	cwd := s.cwd
	if s.sandbox != nil {
		cwd = s.sandbox.CWD()
	} else if cwd == "" {
		cwd, _ = os.Getwd()
	}

	readmeContent := agent.ReadBuildInstructions(cwd, s.sandbox)
	var allTestOutput strings.Builder

	// Run sub-agents sequentially: blackbox first, then whitebox
	for _, entry := range testPlan {
		var a6 *agent.Agent6
		switch entry.Scope {
		case agent.ScopeBlackBox:
			PrintTransition(s.stderr, 6, "QA Tester (Black-Box)")
			a6 = agent.NewAgent6BlackBox(cwd, s.runner.TaskDir(), entry, readmeContent, s.sandbox)
		case agent.ScopeWhiteBox:
			// Check if whitebox was skipped
			if len(entry.Tests) == 1 && strings.Contains(entry.Tests[0], "skipped") {
				fmt.Fprintf(s.stderr, "\n--- Agent 6.2: QA Tester (White-Box) -- skipped (no testable internal logic) ---\n")
				continue
			}
			PrintTransition(s.stderr, 6, "QA Tester (White-Box)")
			agent.ReadSourceForWhiteBox(cwd, &entry, s.sandbox)
			a6 = agent.NewAgent6WhiteBox(cwd, s.runner.TaskDir(), entry, s.sandbox)
		default:
			continue
		}

		testOutput, passed, err := s.runAgent6SubPhase(ctx, a6, handoffPath, taskFileName, iteration)
		allTestOutput.WriteString(testOutput)
		allTestOutput.WriteString("\n")

		if err != nil {
			return allTestOutput.String(), false, err
		}

		if !passed {
			// For blackbox failures, abort immediately -- no point running whitebox
			if entry.Scope == agent.ScopeBlackBox {
				fmt.Fprintf(s.stderr, "\n--- Black-box tests FAILED -- aborting further testing ---\n")
			}
			return allTestOutput.String(), false, nil
		}
	}

	return allTestOutput.String(), true, nil
}

// runAgent7Phase drives Agent 7 (Course Corrector) to review work against the
// original plan and determine if corrections are needed. Single-turn pattern.
func (s *Shell) runAgent7Phase(ctx context.Context, taskFilePath, handoffPath, testOutput string, iteration int) (approved bool, correctionPath string, err error) {
	a7Start := time.Now()
	taskBase := filepath.Base(taskFilePath)
	taskDir := s.runner.TaskDir()
	base := strings.TrimSuffix(taskBase, ".md")
	enrichedFile := filepath.Join(taskDir, base+"-enriched.md")
	changePlanFile := filepath.Join(taskDir, base+"-change-plan.md")

	a7 := agent.NewAgent7(taskDir, enrichedFile, changePlanFile, testOutput, handoffPath)
	s.runner.SwitchAgent(a7)

	s.runner.AppendUserMessage("Review the work against the original plan and determine if corrections are needed.")

	responseBuf, streamErr := s.streamResponse(ctx)
	if streamErr != nil {
		return false, "", streamErr
	}
	s.runner.AppendAssistantMessage(responseBuf)

	// Persist conversation (non-fatal)
	if s.sandbox != nil {
		_, convErr := agent.WriteConversationFile(s.sandbox, taskDir, taskBase, 7, "Course Corrector", iteration, s.runner.History(), time.Since(a7Start))
		if convErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write Agent 7 conversation: %v\n", convErr)
		}
	}

	// Parse verdict
	a7Approved, body := agent.ParseCorrectionVerdict(responseBuf)
	if !a7Approved {
		cp, writeErr := agent.WriteCorrectionFile(s.sandbox, taskDir, taskBase, iteration, body)
		if writeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write correction file: %v\n", writeErr)
		}
		return false, cp, nil
	}
	return true, "", nil
}

// runAgent4FixPhase drives Agent 4 to fix code based on test failure output.
// Returns the path to the new handoff file.
func (s *Shell) runAgent4FixPhase(ctx context.Context, taskFilePath, testOutput string, iteration int, correctionPath string) (string, error) {
	a4FixStart := time.Now()
	cwd := s.cwd
	if s.sandbox != nil {
		cwd = s.sandbox.CWD()
	} else if cwd == "" {
		cwd, _ = os.Getwd()
	}

	a4 := agent.NewAgent4(cwd, s.runner.TaskDir(), taskFilePath, "")
	s.runner.SwitchAgent(a4)

	// Inject fix prompt with test failure output
	fixPrompt := fmt.Sprintf("The following tests failed. Fix the production code so these tests pass.\n\n## Test Output\n```\n%s\n```\n\nFix the production code (not the test files). Output complete file contents.", testOutput)
	if correctionPath != "" {
		corrData, corrErr := os.ReadFile(correctionPath)
		if corrErr == nil {
			fixPrompt += "\n\n## Course Correction (from Agent 7)\n\n" + string(corrData) + "\n\nAddress these corrections in addition to fixing the test failures."
		}
	}
	s.runner.AppendUserMessage(fixPrompt)

	// Autonomous loop
	var allResponses strings.Builder
	for {
		responseBuf, streamErr := s.streamResponse(ctx)
		if streamErr != nil {
			return "", streamErr
		}
		s.runner.AppendAssistantMessage(responseBuf)
		allResponses.WriteString(responseBuf)

		action := a4.HandleResponse(responseBuf)
		if action.Type == agent.ActionComplete {
			break
		}
		s.runner.AppendUserMessage("Continue implementing. Output any remaining code blocks.")
	}

	// Parse and write fixed files
	blocks := agent.ParseCodeBlocks(allResponses.String())
	var handoffPath string
	if len(blocks) > 0 {
		results, blocked := agent.WriteCodeBlocks(s.sandbox, blocks)
		fmt.Fprintln(s.stderr)
		for _, r := range results {
			fmt.Fprintf(s.stderr, "  \u2713 %s (%s)\n", r.Path, r.Action)
		}
		for _, b := range blocked {
			fmt.Fprintf(s.stderr, "  X %s (blocked: %s)\n", b.Path, b.Reason)
		}
		fmt.Fprintln(s.stderr)

		// Verify build before handoff
		var buildErr error
		results, buildErr = s.verifyAgent4Build(ctx, cwd, &allResponses, results)
		if buildErr != nil {
			return "", buildErr
		}

		// Write handoff file
		taskBase := filepath.Base(taskFilePath)
		hp, handoffErr := agent.WriteHandoffFile(s.sandbox, s.runner.TaskDir(), taskBase, results, allResponses.String())
		if handoffErr != nil {
			return "", fmt.Errorf("write handoff: %w", handoffErr)
		}
		handoffPath = hp
	}

	// Persist Agent 4 fix conversation (non-fatal)
	if s.sandbox != nil {
		taskBase := filepath.Base(taskFilePath)
		_, convErr := agent.WriteConversationFile(s.sandbox, s.runner.TaskDir(), taskBase, 4, "Software Engineer", iteration, s.runner.History(), time.Since(a4FixStart))
		if convErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write Agent 4 fix conversation: %v\n", convErr)
		}
	}

	return handoffPath, nil
}

// verifyAgent4Build checks that Agent 4's code compiles before handoff.
// If the build fails, it feeds the error back to Agent 4 and retries up to 3 times.
// Returns updated results (appended with any retry file writes) and error.
func (s *Shell) verifyAgent4Build(ctx context.Context, cwd string, allResponses *strings.Builder, results []agent.FileResult) ([]agent.FileResult, error) {
	if !s.buildVerification {
		return results, nil
	}
	readmeContent := agent.ReadBuildInstructions(cwd, s.sandbox)

	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var buildOutput string
		var buildPassed bool
		var buildErr error

		fmt.Fprintf(s.stderr, "--- Build check (attempt %d/%d) ---\n", attempt, maxAttempts)
		if agent.IsGoProject(cwd) {
			buildOutput, buildPassed, buildErr = agent.RunGoBuild(ctx, cwd, s.stderr)
		} else {
			buildOutput, buildPassed, buildErr = agent.RunBuildAndVerify(ctx, cwd, readmeContent, s.stderr)
		}
		if buildErr != nil {
			return results, buildErr
		}
		if buildPassed {
			fmt.Fprintln(s.stderr, "--- Build passed ---")
			return results, nil
		}

		if attempt == maxAttempts {
			fmt.Fprintf(s.stderr, "--- Build still failing after %d attempts, proceeding to handoff ---\n", maxAttempts)
			return results, nil
		}

		// Build failed -- feed error back to Agent 4
		fmt.Fprintf(s.stderr, "--- Build failed, sending back to Agent 4 ---\n")
		s.runner.AppendUserMessage("The code does not compile. Fix the compilation errors:\n\n```\n" + buildOutput + "\n```\n\nOutput the corrected file contents.")

		var retryResponses strings.Builder
		for {
			responseBuf, streamErr := s.streamResponse(ctx)
			if streamErr != nil {
				return results, streamErr
			}
			s.runner.AppendAssistantMessage(responseBuf)
			retryResponses.WriteString(responseBuf)
			allResponses.WriteString(responseBuf)

			action := s.runner.Active().HandleResponse(responseBuf)
			if action.Type == agent.ActionComplete {
				break
			}
			s.runner.AppendUserMessage("Continue implementing. Output any remaining code blocks.")
		}

		retryBlocks := agent.ParseCodeBlocks(retryResponses.String())
		if len(retryBlocks) > 0 {
			retryResults, retryBlocked := agent.WriteCodeBlocks(s.sandbox, retryBlocks)
			fmt.Fprintln(s.stderr)
			for _, r := range retryResults {
				fmt.Fprintf(s.stderr, "  \u2713 %s (%s)\n", r.Path, r.Action)
			}
			for _, b := range retryBlocked {
				fmt.Fprintf(s.stderr, "  X %s (blocked: %s)\n", b.Path, b.Reason)
			}
			fmt.Fprintln(s.stderr)
			results = append(results, retryResults...)
		}
	}

	return results, nil
}

// runFeedbackLoop orchestrates the Agent 5 (plan) -> Agent 6 (test) -> Agent 7 (review) -> Agent 4 (fix) feedback cycle.
// Agent 5 produces a test plan, Agent 6 sub-agents execute it sequentially (6.1 blackbox, 6.2 whitebox),
// Agent 7 reviews. The loop exits only when all tests pass AND Agent 7 approves.
// Returns whether tests passed, accumulated agent outcomes, and any error.
func (s *Shell) runFeedbackLoop(ctx context.Context, handoffPath, taskFilePath string) (bool, []state.AgentOutcome, error) {
	var outcomes []state.AgentOutcome
	for i := 1; i <= s.maxIterations; i++ {
		fmt.Fprintf(s.stderr, "\n--- Feedback Loop: Iteration %d ---\n\n", i)

		// Agent 5: QA Team Leader -- produces test plan
		PrintTransition(s.stderr, 5, "QA Team Leader")
		testPlan, planErr := s.runAgent5Phase(ctx, handoffPath, filepath.Base(taskFilePath), i)
		if planErr != nil {
			outcomes = append(outcomes, state.AgentOutcome{Number: 5, Name: "QA Team Leader", Status: "failed"})
			return false, outcomes, planErr
		}
		outcomes = append(outcomes, state.AgentOutcome{Number: 5, Name: "QA Team Leader", Status: "success"})

		// Agent 6: QA Tester sub-agents -- execute test plan (6.1 blackbox -> 6.2 whitebox)
		testOutput, passed, testErr := s.runAgent6TestingPhase(ctx, testPlan, handoffPath, filepath.Base(taskFilePath), i)
		if testErr != nil {
			outcomes = append(outcomes, state.AgentOutcome{Number: 6, Name: "QA Tester", Status: "failed"})
			return false, outcomes, testErr
		}

		// Agent 7: Course Corrector -- runs every iteration
		correctionPath := ""
		a7Approved := true
		if s.sandbox != nil {
			PrintTransition(s.stderr, 7, "Course Corrector")
			var a7Err error
			a7Approved, correctionPath, a7Err = s.runAgent7Phase(ctx, taskFilePath, handoffPath, testOutput, i)
			if a7Err != nil {
				// Non-fatal: treat as implicit approval
				fmt.Fprintf(os.Stderr, "Warning: Agent 7 failed: %v\n", a7Err)
				a7Approved = true
				correctionPath = ""
				outcomes = append(outcomes, state.AgentOutcome{Number: 7, Name: "Course Corrector", Status: "failed"})
			} else if a7Approved {
				outcomes = append(outcomes, state.AgentOutcome{Number: 7, Name: "Course Corrector", Status: "success"})
			} else {
				// Agent 7 detected drift -- correction needed, another iteration
				outcomes = append(outcomes, state.AgentOutcome{Number: 7, Name: "Course Corrector", Status: "success"})
			}
		} else {
			// No sandbox -- Agent 7 skipped
			outcomes = append(outcomes, state.AgentOutcome{Number: 7, Name: "Course Corrector", Status: "skipped"})
		}

		// Exit: tests pass AND Agent 7 approves
		if passed && a7Approved {
			s.printPipelineComplete()
			outcomes = append(outcomes, state.AgentOutcome{Number: 6, Name: "QA Tester", Status: "success"})
			return true, outcomes, nil
		}

		// Continue: either tests failed or Agent 7 detected drift
		PrintTransition(s.stderr, 4, "Software Engineer")
		var fixErr error
		handoffPath, fixErr = s.runAgent4FixPhase(ctx, taskFilePath, testOutput, i+1, correctionPath)
		if fixErr != nil {
			outcomes = append(outcomes, state.AgentOutcome{Number: 6, Name: "QA Tester", Status: "failed"})
			return false, outcomes, fixErr
		}
	}

	// Max iterations exceeded
	fmt.Fprintf(s.stderr, "\n--- Feedback Loop: max iterations (%d) reached ---\n", s.maxIterations)
	return false, outcomes, fmt.Errorf("feedback loop exceeded %d iterations", s.maxIterations)
}

// printPipelineComplete prints the pipeline result banner.
func (s *Shell) printPipelineComplete() {
	fmt.Fprintln(s.stderr, "\n--- Pipeline Complete: PASS ---")
}

// isCompilationError checks if go test output indicates a build/compilation error
// rather than a test failure. Go prefixes build errors with "# " followed by package path.
func isCompilationError(testOutput string) bool {
	for _, line := range strings.Split(testOutput, "\n") {
		if strings.HasPrefix(line, "# ") {
			return true
		}
	}
	return false
}

// getLastTaskFile finds the most recently created task prompt file in the task
// directory (the last .md file that isn't a handoff file, sorted by name).
func getLastTaskFile(taskDir string) string {
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return ""
	}

	var taskFiles []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, "-task.md") {
			taskFiles = append(taskFiles, name)
		}
	}

	if len(taskFiles) == 0 {
		return ""
	}

	sort.Strings(taskFiles)
	return filepath.Join(taskDir, taskFiles[len(taskFiles)-1])
}

// detectResumeStage determines which pipeline stage to resume from based on
// which artifact files exist for a given task. Returns a human-readable stage name.
func detectResumeStage(taskDir, taskBase string) string {
	if _, err := os.Stat(filepath.Join(taskDir, taskBase+"-handoff.md")); err == nil {
		return "Agent 5: QA Team Leader"
	}
	if _, err := os.Stat(filepath.Join(taskDir, taskBase+"-change-plan.md")); err == nil {
		return "Agent 4: Software Engineer"
	}
	if _, err := os.Stat(filepath.Join(taskDir, taskBase+"-enriched.md")); err == nil {
		return "Agent 3: Software Architect"
	}
	return "Agent 2: Prompt Engineer"
}

// resumeLastTask finds the last interrupted/failed task and resumes the pipeline.
func (s *Shell) resumeLastTask(ctx context.Context) error {
	idx, _ := state.LoadHistory(filepath.Join(s.stateDir, "history.json"))
	if len(idx.Records) == 0 {
		fmt.Fprintln(s.stderr, "No tasks to resume.")
		return nil
	}
	last := idx.Records[len(idx.Records)-1]
	if last.Outcome != "interrupted" && last.Outcome != "failure" {
		fmt.Fprintln(s.stderr, "Last task completed successfully. Nothing to resume.")
		return nil
	}

	taskFile := filepath.Join(s.stateDir, last.TaskFile)
	if _, err := os.Stat(taskFile); err != nil {
		fmt.Fprintf(s.stderr, "Task file not found: %s\n", taskFile)
		return nil
	}

	taskBase := strings.TrimSuffix(filepath.Base(taskFile), ".md")
	taskDir := filepath.Dir(taskFile)

	// Determine resume point based on existing artifacts
	handoffFile := filepath.Join(taskDir, taskBase+"-handoff.md")
	changePlanFile := filepath.Join(taskDir, taskBase+"-change-plan.md")
	enrichedFile := filepath.Join(taskDir, taskBase+"-enriched.md")

	startTime := time.Now()
	sessionID := state.GenerateSessionID()
	outcome := (*string)(nil)
	var agentOutcomes []state.AgentOutcome

	defer func() {
		if outcome == nil && s.stateDir != "" {
			s.recordOutcome(sessionID, taskFile, "interrupted", "", startTime, nil, agentOutcomes)
		}
	}()

	cwd := s.cwd
	if s.sandbox != nil {
		cwd = s.sandbox.CWD()
	} else if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Always mark Agent 1 as done (task file exists)
	agentOutcomes = append(agentOutcomes, state.AgentOutcome{
		Number: 1, Name: "Systems Engineer", Status: "success",
	})

	// Resume from handoff → run feedback loop (Agent 5)
	if _, err := os.Stat(handoffFile); err == nil {
		fmt.Fprintf(s.stderr, "\nResuming from Agent 5 (QA Team Leader)...\n")
		agentOutcomes = append(agentOutcomes,
			state.AgentOutcome{Number: 2, Name: "Prompt Engineer", Status: "success"},
			state.AgentOutcome{Number: 3, Name: "Software Architect", Status: "success"},
			state.AgentOutcome{Number: 4, Name: "Software Engineer", Status: "success"},
		)
		passed, loopOutcomes, err := s.runFeedbackLoop(ctx, handoffFile, taskFile)
		agentOutcomes = append(agentOutcomes, loopOutcomes...)
		PrintPipelineSummary(s.stderr, agentOutcomes)
		if err != nil {
			return err
		}
		if passed {
			o := "success"
			outcome = &o
			s.recordOutcome(sessionID, taskFile, "success", "pass", startTime, nil, agentOutcomes)
		} else {
			o := "failure"
			outcome = &o
			s.recordOutcome(sessionID, taskFile, "failure", "fail", startTime, nil, agentOutcomes)
		}
		return nil
	}

	// Resume from change plan → run Agent 4 + Agent 5/6/7
	inputForAgent4 := taskFile
	if _, err := os.Stat(enrichedFile); err == nil {
		inputForAgent4 = enrichedFile
	}

	if _, err := os.Stat(changePlanFile); err == nil {
		fmt.Fprintf(s.stderr, "\nResuming from Agent 4 (Software Engineer)...\n")
		agentOutcomes = append(agentOutcomes,
			state.AgentOutcome{Number: 2, Name: "Prompt Engineer", Status: "success"},
			state.AgentOutcome{Number: 3, Name: "Software Architect", Status: "success"},
		)
		return s.runPipelineFromAgent4(ctx, taskFile, changePlanFile, sessionID, startTime, &outcome, &agentOutcomes)
	}

	// Resume from enriched → run Agent 3 + Agent 4 + Agent 5/6/7
	if _, err := os.Stat(enrichedFile); err == nil {
		fmt.Fprintf(s.stderr, "\nResuming from Agent 3 (Software Architect)...\n")
		agentOutcomes = append(agentOutcomes,
			state.AgentOutcome{Number: 2, Name: "Prompt Engineer", Status: "success"},
		)
		PrintTransition(s.stderr, 3, "Software Architect")
		changePlanPath, a3Err := s.runAgent3Phase(ctx, enrichedFile)
		if a3Err != nil {
			fmt.Fprintf(s.stderr, "Warning: Agent 3 failed: %v\n", a3Err)
			agentOutcomes = append(agentOutcomes, state.AgentOutcome{Number: 3, Name: "Software Architect", Status: "failed"})
			changePlanPath = ""
		} else {
			agentOutcomes = append(agentOutcomes, state.AgentOutcome{Number: 3, Name: "Software Architect", Status: "success"})
		}
		return s.runPipelineFromAgent4(ctx, inputForAgent4, changePlanPath, sessionID, startTime, &outcome, &agentOutcomes)
	}

	// No enriched file → run full pipeline from Agent 2
	fmt.Fprintf(s.stderr, "\nResuming from Agent 2 (Prompt Engineer)...\n")
	return s.runPipeline(ctx, taskFile)
}

// runPipelineFromAgent4 runs Agent 4 → Agent 5/6/7 feedback loop.
// Shared between runPipeline and resumeLastTask.
func (s *Shell) runPipelineFromAgent4(ctx context.Context, taskFilePath, changePlanPath, sessionID string, startTime time.Time, outcome **string, agentOutcomes *[]state.AgentOutcome) error {
	cwd := s.cwd
	if s.sandbox != nil {
		cwd = s.sandbox.CWD()
	} else if cwd == "" {
		cwd, _ = os.Getwd()
	}

	PrintTransition(s.stderr, 4, "Software Engineer")
	a4 := agent.NewAgent4(cwd, s.runner.TaskDir(), taskFilePath, changePlanPath)
	s.runner.SwitchAgent(a4)

	taskContent, err := os.ReadFile(taskFilePath)
	if err != nil {
		return fmt.Errorf("read task file: %w", err)
	}
	s.runner.AppendUserMessage("Here is the task prompt:\n\n" + string(taskContent))

	var allResponses strings.Builder
	for {
		responseBuf, streamErr := s.streamResponse(ctx)
		if streamErr != nil {
			return nil
		}
		s.runner.AppendAssistantMessage(responseBuf)
		allResponses.WriteString(responseBuf)

		action := a4.HandleResponse(responseBuf)
		if action.Type == agent.ActionComplete {
			break
		}
		s.runner.AppendUserMessage("Continue implementing. Output any remaining code blocks.")
	}

	blocks := agent.ParseCodeBlocks(allResponses.String())
	var handoffPath string
	var results []agent.FileResult
	if len(blocks) > 0 {
		var blocked []sandbox.BlockedFile
		results, blocked = agent.WriteCodeBlocks(s.sandbox, blocks)
		fmt.Fprintln(s.stderr)
		for _, r := range results {
			fmt.Fprintf(s.stderr, "  \u2713 %s (%s)\n", r.Path, r.Action)
		}
		for _, b := range blocked {
			fmt.Fprintf(s.stderr, "  X %s (blocked: %s)\n", b.Path, b.Reason)
		}
		fmt.Fprintln(s.stderr)

		// Verify build before handoff
		var buildErr error
		results, buildErr = s.verifyAgent4Build(ctx, cwd, &allResponses, results)
		if buildErr != nil {
			return buildErr
		}

		taskBase := filepath.Base(taskFilePath)
		hp, handoffErr := agent.WriteHandoffFile(s.sandbox, s.runner.TaskDir(), taskBase, results, allResponses.String())
		if handoffErr != nil {
			ColorError.Fprintf(s.stderr, "Error writing handoff: %s\n", handoffErr)
		} else {
			handoffPath = hp
			fmt.Fprintf(s.stderr, "Handoff written to %s\n", handoffPath)
		}
	} else {
		fmt.Fprintln(s.stderr, "No code blocks produced. Agent 4 may not have used the required format:")
		fmt.Fprintln(s.stderr, "  ```go:path/to/file.go")
		fmt.Fprintln(s.stderr, "  [file content]")
		fmt.Fprintln(s.stderr, "  ```")
	}

	*agentOutcomes = append(*agentOutcomes, state.AgentOutcome{
		Number: 4, Name: "Software Engineer", Status: "success",
	})

	var filesModified []string
	for _, r := range results {
		filesModified = append(filesModified, r.Path)
	}

	if handoffPath != "" {
		passed, loopOutcomes, err := s.runFeedbackLoop(ctx, handoffPath, taskFilePath)
		*agentOutcomes = append(*agentOutcomes, loopOutcomes...)
		PrintPipelineSummary(s.stderr, *agentOutcomes)
		if err != nil {
			return err
		}
		if passed {
			o := "success"
			*outcome = &o
			s.recordOutcome(sessionID, taskFilePath, "success", "pass", startTime, filesModified, *agentOutcomes)
		} else {
			o := "failure"
			*outcome = &o
			s.recordOutcome(sessionID, taskFilePath, "failure", "fail", startTime, filesModified, *agentOutcomes)
		}
	} else {
		*agentOutcomes = append(*agentOutcomes, state.AgentOutcome{
			Number: 5, Name: "QA Team Leader", Status: "skipped",
		})
		PrintPipelineSummary(s.stderr, *agentOutcomes)
		o := "success"
		*outcome = &o
		s.recordOutcome(sessionID, taskFilePath, "success", "", startTime, filesModified, *agentOutcomes)
	}
	return nil
}
