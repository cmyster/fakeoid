package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmyster/fakeoid/internal/agent"
	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/cmyster/fakeoid/internal/server"
	"github.com/cmyster/fakeoid/internal/state"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintPipelineSummary_AllRan(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	var buf bytes.Buffer
	outcomes := []state.AgentOutcome{
		{Number: 1, Name: "Systems Engineer", Status: "success"},
		{Number: 2, Name: "Prompt Engineer", Status: "success"},
		{Number: 3, Name: "Software Architect", Status: "success"},
		{Number: 4, Name: "Software Engineer", Status: "success"},
		{Number: 5, Name: "QE Engineer", Status: "success"},
	}
	PrintPipelineSummary(&buf, outcomes)
	assert.Contains(t, buf.String(), "5/5 agents ran")
	assert.NotContains(t, buf.String(), "skipped")
}

func TestPrintPipelineSummary_WithSkipped(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	var buf bytes.Buffer
	outcomes := []state.AgentOutcome{
		{Number: 1, Name: "Systems Engineer", Status: "success"},
		{Number: 2, Name: "Prompt Engineer", Status: "failed"},
		{Number: 3, Name: "Software Architect", Status: "skipped"},
		{Number: 4, Name: "Software Engineer", Status: "success"},
		{Number: 5, Name: "QE Engineer", Status: "success"},
	}
	PrintPipelineSummary(&buf, outcomes)
	assert.Contains(t, buf.String(), "4/5 agents ran")
	assert.Contains(t, buf.String(), "skipped: Agent 3")
}

// mockClient implements ChatClient for testing.
type mockClient struct {
	streamFn func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error)
}

func (m *mockClient) StreamChatCompletion(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
	return m.streamFn(ctx, msgs, onToken)
}

// newTestShell creates a Shell with piped stdin for testing (no runner).
func newTestShell(t *testing.T, input string, client ChatClient, ctxSize int) (*Shell, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	sh, err := New(client, ctxSize, "test-model", "test-gpu", "", WithStdin(io.NopCloser(strings.NewReader(input))), WithStderr(out))
	require.NoError(t, err)
	return sh, out
}

// newTestShellWithRunner creates a Shell with piped stdin and an AgentRunner for testing.
func newTestShellWithRunner(t *testing.T, input string, client ChatClient, ctxSize int, runner *agent.AgentRunner) (*Shell, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	sh, err := New(client, ctxSize, "test-model", "test-gpu", "",
		WithStdin(io.NopCloser(strings.NewReader(input))),
		WithStdout(stdout),
		WithStderr(stderr),
		WithAgentRunner(runner),
	)
	require.NoError(t, err)
	return sh, stdout, stderr
}

func TestShell_ExitOnInterrupt(t *testing.T) {
	// When readline gets EOF (simulated by closing stdin), shell exits cleanly.
	// We use empty input which will cause EOF.
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			return server.StreamResult{}, nil
		},
	}

	sh, _ := newTestShell(t, "", client, 8192)
	defer sh.Close()

	err := sh.Run(context.Background())
	assert.NoError(t, err)
}

func TestShell_ExitOnEOF(t *testing.T) {
	// Empty input causes io.EOF from readline, shell should exit cleanly.
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			return server.StreamResult{}, nil
		},
	}

	sh, _ := newTestShell(t, "", client, 8192)
	defer sh.Close()

	err := sh.Run(context.Background())
	assert.NoError(t, err)
}

func TestShell_SendsUserMessageToClient(t *testing.T) {
	var capturedMsgs []server.Message
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			capturedMsgs = msgs
			onToken("response")
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	// Provide one line of input, then EOF
	sh, _ := newTestShell(t, "hello world\n", client, 8192)
	defer sh.Close()

	err := sh.Run(context.Background())
	assert.NoError(t, err)
	require.NotEmpty(t, capturedMsgs)
	assert.Equal(t, "user", capturedMsgs[len(capturedMsgs)-1].Role)
	assert.Equal(t, "hello world", capturedMsgs[len(capturedMsgs)-1].Content)
}

func TestShell_MultiTurnConversation(t *testing.T) {
	callCount := 0
	var lastMsgCount int
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			lastMsgCount = len(msgs)
			onToken("reply")
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	// Two turns separated by empty line (multi-line delimiter)
	sh, _ := newTestShell(t, "first\n\nsecond\n", client, 8192)
	defer sh.Close()

	err := sh.Run(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
	// Second call should have: user1, assistant1, user2 = 3 messages
	assert.Equal(t, 3, lastMsgCount)
}

func TestShell_TrimHistory(t *testing.T) {
	sh := &Shell{
		ctxSize:    1000,
		usedTokens: 900, // 90% > 80% threshold
		history: []server.Message{
			{Role: "user", Content: strings.Repeat("a", 100)},
			{Role: "assistant", Content: strings.Repeat("b", 100)},
			{Role: "user", Content: "recent question"},
			{Role: "assistant", Content: "recent answer"},
		},
	}

	sh.trimHistory()

	// Should have dropped the oldest pair
	assert.Equal(t, 2, len(sh.history))
	assert.Equal(t, "recent question", sh.history[0].Content)
}

func TestShell_MultiLineInput(t *testing.T) {
	// Multi-line: backslash continuation joins lines
	var capturedContent string
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			capturedContent = msgs[len(msgs)-1].Content
			onToken("ok")
			return server.StreamResult{Usage: server.Usage{TotalTokens: 50}}, nil
		},
	}

	// First line ends with backslash to trigger continuation, second line terminates
	sh, _ := newTestShell(t, "line one\\\nline two\n", client, 8192)
	defer sh.Close()

	err := sh.Run(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "line one\nline two", capturedContent)
}

func TestShell_MultiLineInputSingleLine(t *testing.T) {
	// Single-line input (one line then EOF) still works as before
	var capturedContent string
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			capturedContent = msgs[len(msgs)-1].Content
			onToken("ok")
			return server.StreamResult{Usage: server.Usage{TotalTokens: 50}}, nil
		},
	}

	// Single line then EOF (no empty line terminator needed)
	sh, _ := newTestShell(t, "hello\n", client, 8192)
	defer sh.Close()

	err := sh.Run(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "hello", capturedContent)
}

func TestShell_StreamCancellationReturnsToPrompt(t *testing.T) {
	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			if callCount == 1 {
				// Simulate cancellation
				return server.StreamResult{}, context.Canceled
			}
			onToken("success")
			return server.StreamResult{Usage: server.Usage{TotalTokens: 50}}, nil
		},
	}

	// Two inputs separated by empty line: first will get "cancelled", second should succeed
	sh, _ := newTestShell(t, "first\n\nsecond\n", client, 8192)
	defer sh.Close()

	err := sh.Run(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestShell_TriggerGoWritesTaskFile(t *testing.T) {
	// Create a temp directory with a minimal Go project so go test can run
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	// Set up go.mod so go test works in the temp dir
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "go.mod"),
		[]byte("module example.com/test\n\ngo 1.21\n"), 0o644))

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			// Detect which agent is calling by checking the system prompt
			sysPrompt := ""
			if len(msgs) > 0 && msgs[0].Role == "system" {
				sysPrompt = msgs[0].Content
			}
			switch {
			case callCount == 1:
				// Agent 1: normal conversation response
				onToken("I understand your task.")
			case callCount == 2:
				// Agent 1: generate the task prompt
				onToken("# Task: Build a feature\n\n## Description\nBuild it.")
			case strings.Contains(sysPrompt, "Course Corrector"):
				// Agent 6: approve
				onToken("## APPROVED\n\nWork matches the plan.")
			case strings.Contains(sysPrompt, "QE Engineer"):
				// Agent 5: produce a valid test file
				onToken("```go:hello_test.go\npackage main\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {\n\tif hello() != \"hello\" {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n```")
			default:
				// Agent 2/3/4: produce a valid code file
				onToken("```go:hello.go\npackage main\n\nfunc hello() string { return \"hello\" }\n```")
			}
			return server.StreamResult{Usage: server.Usage{TotalTokens: 200}}, nil
		},
	}

	runner := agent.NewAgentRunner(taskDir)
	a1 := agent.NewAgent1(cwd, taskDir)
	runner.ActivateAgent(a1)

	// First turn: "build a feature" -> LLM responds -> turnCount=1
	// Second turn: "go" -> trigger -> generate task file -> confirming
	// Third turn: "go" -> complete -> pipeline runs
	sh, _, stderr := newTestShellWithRunner(t, "build a feature\ngo\ngo\n", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	err = sh.Run(context.Background())
	assert.NoError(t, err)

	// Verify task file was written (may also have change-plan file from Agent 3)
	entries, readErr := os.ReadDir(taskDir)
	require.NoError(t, readErr)
	assert.GreaterOrEqual(t, len(entries), 1, "expected at least one task file")

	// Verify stderr contains key messages
	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "Task prompt written to")
	assert.Contains(t, stderrStr, "Say 'go' to hand off, or add corrections.")
	assert.Contains(t, stderrStr, "Handing off to Agent 4: Software Engineer")
}

func TestShell_TriggerIgnoredInSentence(t *testing.T) {
	taskDir := filepath.Join(t.TempDir(), ".fakeoid", "tasks")

	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			onToken("got it")
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	runner := agent.NewAgentRunner(taskDir)
	a1 := agent.NewAgent1(t.TempDir(), taskDir)
	runner.ActivateAgent(a1)

	// "let's go ahead" should NOT trigger task generation
	sh, _, _ := newTestShellWithRunner(t, "describe task\n\nlet's go ahead\n", client, 8192, runner)
	defer sh.Close()

	err := sh.Run(context.Background())
	assert.NoError(t, err)

	// Both inputs should be treated as normal conversation
	assert.Equal(t, 2, callCount)

	// No task file should be created
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(taskDir)
		assert.Empty(t, entries, "no task file should be created for 'let's go ahead'")
	}
}

func TestShell_Agent4_Activation(t *testing.T) {
	// Verify that after Agent 1 auto-detects a task prompt, the pipeline runs
	// through Agent 2 -> Agent 3 -> Agent 4 -> Agent 5 and writes code files.
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	// Set up go.mod so go test works
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))

	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			sysPrompt := ""
			if len(msgs) > 0 && msgs[0].Role == "system" {
				sysPrompt = msgs[0].Content
			}
			switch {
			case callCount == 1:
				// Agent 1: produces complete task prompt (auto-proceed)
				onToken("# Task: Build a feature\n\n## Description\nBuild it.\n\n## Affected Files\n- main.go\n\n## Expected Tests\n- TestMain\n\n## Acceptance Criteria\n- Compiles\n\n## Context\nN/A")
			case strings.Contains(sysPrompt, "Course Corrector"):
				// Agent 6: approve
				onToken("## APPROVED\n\nWork matches the plan.")
			case strings.Contains(sysPrompt, "QE Engineer"):
				// Agent 5: produce a passing test
				onToken("```go:main_test.go\npackage main\n\nimport \"testing\"\n\nfunc TestMain2(t *testing.T) {}\n```\n")
			default:
				// Agents 2/3/4: produce a code block
				onToken("```go:main.go\npackage main\n\nfunc main() {}\n```\n")
			}
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	runner := agent.NewAgentRunner(taskDir)
	a1 := agent.NewAgent1(cwd, taskDir)
	runner.ActivateAgent(a1)

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	// Flow: describe task -> auto-proceed through pipeline
	sh, _, stderr := newTestShellWithRunner(t, "build a feature\n", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	err = sh.Run(context.Background())
	assert.NoError(t, err)

	// Agent 2 + Agent 3 + Agent 4 should have been called (callCount >= 4)
	assert.GreaterOrEqual(t, callCount, 4, "Agents 2, 3, and 4 should make LLM calls")

	// Verify file was written to CWD
	_, statErr := os.Stat(filepath.Join(cwd, "main.go"))
	assert.NoError(t, statErr, "main.go should be written to CWD")

	content, readErr := os.ReadFile(filepath.Join(cwd, "main.go"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "package main")

	// Verify stderr shows file confirmation and Agent 5 transition
	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "main.go")
	assert.Contains(t, stderrStr, "created")
	assert.Contains(t, stderrStr, "Agent 5")
}

func TestShell_Agent4_StreamingVisible(t *testing.T) {
	// Verify Agent 4's streaming output appears in stdout
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	// Set up go.mod so go test works
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))

	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			sysPrompt := ""
			if len(msgs) > 0 && msgs[0].Role == "system" {
				sysPrompt = msgs[0].Content
			}
			switch {
			case callCount == 1:
				onToken("I understand.")
			case callCount == 2:
				onToken("# Task\n\nDo it.")
			case strings.Contains(sysPrompt, "Course Corrector"):
				onToken("## APPROVED\n\nWork matches the plan.")
			case strings.Contains(sysPrompt, "QE Engineer"):
				onToken("```go:hello_test.go\npackage main\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {}\n```\n")
			case strings.Contains(sysPrompt, "Software Engineer"):
				onToken("Writing code now.\n\n```go:hello.go\npackage main\n```\n")
			default:
				// Agents 2/3: produce code blocks
				onToken("```go:hello.go\npackage main\n```\n")
			}
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	runner := agent.NewAgentRunner(taskDir)
	a1 := agent.NewAgent1(cwd, taskDir)
	runner.ActivateAgent(a1)

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	sh, stdout, _ := newTestShellWithRunner(t, "do a thing\ngo\ngo\n", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	err = sh.Run(context.Background())
	assert.NoError(t, err)

	// Agent 4 streaming output should be visible in stdout
	assert.Contains(t, stdout.String(), "Writing code now.")
}

func TestShell_Agent4_NoFilesOnCancel(t *testing.T) {
	// If stream errors during Agent 4, no code files should be written to CWD.
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			sysPrompt := ""
			if len(msgs) > 0 && msgs[0].Role == "system" {
				sysPrompt = msgs[0].Content
			}
			switch {
			case callCount == 1:
				onToken("Got it.")
			case callCount == 2:
				onToken("# Task\n\nBuild.")
			case strings.Contains(sysPrompt, "Software Engineer"):
				// Agent 4: simulate cancellation
				return server.StreamResult{}, fmt.Errorf("stream cancelled")
			default:
				// Agents 2/3: produce something to move along
				onToken("```go:dummy.go\npackage main\n```\n")
			}
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	runner := agent.NewAgentRunner(taskDir)
	a1 := agent.NewAgent1(cwd, taskDir)
	runner.ActivateAgent(a1)

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	sh, _, _ := newTestShellWithRunner(t, "build it\ngo\ngo\n", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	err = sh.Run(context.Background())
	assert.NoError(t, err)

	// No code files should exist in CWD (besides .fakeoid dir from task file creation)
	entries, readErr := os.ReadDir(cwd)
	require.NoError(t, readErr)
	var nonTask []os.DirEntry
	for _, e := range entries {
		if e.Name() != ".fakeoid" {
			nonTask = append(nonTask, e)
		}
	}
	assert.Empty(t, nonTask, "no files should be written when Agent 4 stream is cancelled")
}

// --- Feedback Loop Tests (Plan 07-02) ---

func TestFeedbackLoop_PassesOnFirstIteration(t *testing.T) {
	// When tests pass on first iteration, feedback loop prints PASS and stops.
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	// Write a handoff file for Agent 5 to read
	require.NoError(t, os.MkdirAll(taskDir, 0o755))
	handoffContent := "# Handoff: Code Changes\n\n## Files Modified/Created\n\n- main.go (created)\n"
	handoffPath := filepath.Join(taskDir, "task-001-handoff.md")
	require.NoError(t, os.WriteFile(handoffPath, []byte(handoffContent), 0o644))

	// Write a valid go module and source file so go test can run
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "main.go"), []byte("package main\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644))

	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			if callCount == 1 {
				// Agent 5 response: produce a test file that passes
				onToken("```go:main_test.go\npackage main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1,2) != 3 { t.Fatal(\"wrong\") }\n}\n```\n")
			} else {
				// Agent 6 response: approve
				onToken("## APPROVED\n\nThe implementation matches the plan.")
			}
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	runner := agent.NewAgentRunner(taskDir)

	sh, _, stderr := newTestShellWithRunner(t, "", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	passed, loopOutcomes, err := sh.runFeedbackLoop(context.Background(), handoffPath, filepath.Join(taskDir, "task-001.md"))
	assert.NoError(t, err)
	assert.True(t, passed)
	// Find the final Agent 5 outcome
	var a5Status string
	for _, o := range loopOutcomes {
		if o.Number == 5 {
			a5Status = o.Status
		}
	}
	assert.Equal(t, "success", a5Status)
	// Should also have Agent 6 outcome
	var a6Status string
	for _, o := range loopOutcomes {
		if o.Number == 6 {
			a6Status = o.Status
		}
	}
	assert.Equal(t, "success", a6Status)

	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "Feedback Loop: Iteration 1")
	assert.Contains(t, stderrStr, "Pipeline Complete: PASS")
	assert.NotContains(t, stderrStr, "Iteration 2")
}

func TestFeedbackLoop_RetriesOnFailure(t *testing.T) {
	// When tests fail, feedback loop retries. Use context cancellation to stop.
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	require.NoError(t, os.MkdirAll(taskDir, 0o755))
	handoffContent := "# Handoff\n\n## Files Modified/Created\n\n- main.go (created)\n"
	handoffPath := filepath.Join(taskDir, "task-001-handoff.md")
	require.NoError(t, os.WriteFile(handoffPath, []byte(handoffContent), 0o644))

	taskFilePath := filepath.Join(taskDir, "task-001.md")
	require.NoError(t, os.WriteFile(taskFilePath, []byte("# Task\n\nBuild it."), 0o644))

	// Write a valid go module
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "main.go"), []byte("package main\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644))

	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			// Flow: Agent5(1) -> Agent6(2) -> Agent4fix(3) -> Agent5(4) -> Agent6(5, cancel)
			// Cancel at call 6 to fail during iteration 2's Agent 6
			if callCount > 5 {
				return server.StreamResult{}, fmt.Errorf("stream cancelled")
			}
			if callCount == 2 || callCount == 5 {
				// Agent 6 response: correction needed (tests failed, so correction expected)
				onToken("## CORRECTION NEEDED\n\nThe code needs fixes.")
			} else {
				// Agent 5 or Agent 4 response: produce a failing test / fix code
				onToken("```go:main_test.go\npackage main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1,2) != 999 { t.Fatal(\"wrong\") }\n}\n```\n")
			}
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	runner := agent.NewAgentRunner(taskDir)

	sh, _, stderr := newTestShellWithRunner(t, "", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	passed, loopOutcomes, err := sh.runFeedbackLoop(context.Background(), handoffPath, taskFilePath)
	assert.False(t, passed)
	// At least one outcome should be present
	assert.NotEmpty(t, loopOutcomes)

	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "Iteration 1")
	assert.Contains(t, stderrStr, "Iteration 2")
}

func TestFeedbackLoop_PrintsTransitions(t *testing.T) {
	// Verify feedback loop alternates PrintTransition between Agent 5 and Agent 4.
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	require.NoError(t, os.MkdirAll(taskDir, 0o755))
	handoffContent := "# Handoff\n\n## Files Modified/Created\n\n- main.go (created)\n"
	handoffPath := filepath.Join(taskDir, "task-001-handoff.md")
	require.NoError(t, os.WriteFile(handoffPath, []byte(handoffContent), 0o644))

	taskFilePath := filepath.Join(taskDir, "task-001.md")
	require.NoError(t, os.WriteFile(taskFilePath, []byte("# Task\n\nBuild it."), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(cwd, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "main.go"), []byte("package main\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644))

	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			// Flow: Agent5(1) -> Agent6(2) -> Agent4(3) -> cancel at Agent5(4)
			if callCount > 3 {
				return server.StreamResult{}, fmt.Errorf("stream cancelled")
			}
			if callCount == 2 {
				// Agent 6 response
				onToken("## CORRECTION NEEDED\n\nNeeds fixes.")
			} else {
				onToken("```go:main_test.go\npackage main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1,2) != 999 { t.Fatal(\"wrong\") }\n}\n```\n")
			}
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	runner := agent.NewAgentRunner(taskDir)

	sh, _, stderr := newTestShellWithRunner(t, "", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	_, _, _ = sh.runFeedbackLoop(context.Background(), handoffPath, taskFilePath)

	stderrStr := stderr.String()
	// Agent 5 transition in each iteration
	assert.Contains(t, stderrStr, "Agent 5: QE Engineer")
	// Agent 6 transition
	assert.Contains(t, stderrStr, "Agent 6: Course Corrector")
	// Agent 4 transition for fix attempts
	assert.Contains(t, stderrStr, "Agent 4: Software Engineer")
}

func TestPipeline_ReturnsToAgent1AfterCompletion(t *testing.T) {
	// After pipeline completes, Run() should reactivate Agent 1 and continue REPL.
	// With auto-proceed, Agent 1 detects a complete task prompt and runs the pipeline
	// without requiring "go" triggers.
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			switch {
			case callCount == 1:
				// Agent 1: produces complete task prompt (auto-proceed triggers)
				onToken("# Task: Build a thing\n\n## Description\nBuild the thing.\n\n## Affected Files\n- main.go\n\n## Expected Tests\n- TestAdd\n\n## Acceptance Criteria\n- It works\n\n## Context\nN/A")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case callCount == 2:
				// Agent 2: enriched prompt (single turn, returns DONE)
				onToken("DONE\n\n# Enriched Task\n\n## Description\nBuild the thing.\n\n## Acceptance Criteria\n- It works")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case callCount == 3:
				// Agent 3: change plan
				onToken("# Change Plan\n\n## Changes\n### CREATE main.go\n")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case callCount == 4:
				// Agent 4: code with code blocks
				onToken("```go:main.go\npackage main\n\nfunc Add(a, b int) int { return a + b }\n```\n")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case callCount == 5:
				// Agent 5: tests
				onToken("```go:main_test.go\npackage main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1,2) != 3 { t.Fatal(\"wrong\") }\n}\n```\n")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case callCount == 6:
				// Agent 6: approve
				onToken("## APPROVED\n\nWork matches the plan.")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case callCount == 7:
				// Back to Agent 1 after pipeline
				onToken("Ready for next task.")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			}
			return server.StreamResult{}, nil
		},
	}

	runner := agent.NewAgentRunner(taskDir)
	a1 := agent.NewAgent1(cwd, taskDir)
	runner.ActivateAgent(a1)

	// Write go.mod in cwd for go test to work
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	// Flow: describe task -> auto-proceed through pipeline -> "hello" (back to Agent 1)
	sh, _, stderr := newTestShellWithRunner(t, "build a thing\nhello\n", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	err = sh.Run(context.Background())
	assert.NoError(t, err)

	// After pipeline, should have returned to Agent 1
	_, isAgent1 := runner.Active().(*agent.Agent1)
	assert.True(t, isAgent1, "should be Agent 1 after pipeline completes")

	// Should have had at least 7 calls (Agent1, Agent2, Agent3, Agent4, Agent5, Agent6, Agent1 again)
	assert.GreaterOrEqual(t, callCount, 7)

	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "Pipeline Complete: PASS")
}

func TestAgent5Phase_WritesTestFiles(t *testing.T) {
	// Verify runAgent5Phase writes test files to disk via ParseCodeBlocks + WriteCodeBlocks.
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	require.NoError(t, os.MkdirAll(taskDir, 0o755))
	handoffContent := "# Handoff\n\n## Files Modified/Created\n\n- main.go (created)\n"
	handoffPath := filepath.Join(taskDir, "task-001-handoff.md")
	require.NoError(t, os.WriteFile(handoffPath, []byte(handoffContent), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(cwd, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "main.go"), []byte("package main\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644))

	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			onToken("```go:main_test.go\npackage main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1,2) != 3 { t.Fatal(\"wrong\") }\n}\n```\n")
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	runner := agent.NewAgentRunner(taskDir)

	sh, _, _ := newTestShellWithRunner(t, "", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	testOutput, passed, runErr := sh.runAgent5Phase(context.Background(), handoffPath, "task-001.md", 1)
	assert.NoError(t, runErr)
	assert.True(t, passed)
	assert.Contains(t, testOutput, "PASS")

	// Test file should exist on disk
	_, statErr := os.Stat(filepath.Join(cwd, "main_test.go"))
	assert.NoError(t, statErr, "test file should be written to disk")
}

func TestAgent4FixPhase_InjectsFailureOutput(t *testing.T) {
	// Verify runAgent4FixPhase injects test output as user message and writes fixes.
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	require.NoError(t, os.MkdirAll(taskDir, 0o755))
	taskFilePath := filepath.Join(taskDir, "task-001.md")
	require.NoError(t, os.WriteFile(taskFilePath, []byte("# Task\n\nBuild it."), 0o644))

	var capturedMsgs []server.Message
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			capturedMsgs = msgs
			onToken("```go:main.go\npackage main\n\nfunc Add(a, b int) int { return a + b }\n```\n")
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	runner := agent.NewAgentRunner(taskDir)

	sh, _, _ := newTestShellWithRunner(t, "", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	testOutput := "FAIL: TestAdd - expected 3 got 999"
	handoffPath, err := sh.runAgent4FixPhase(context.Background(), taskFilePath, testOutput, 2, "")
	assert.NoError(t, err)
	assert.NotEmpty(t, handoffPath)

	// Check that user message contained the test failure output
	found := false
	for _, m := range capturedMsgs {
		if m.Role == "user" && strings.Contains(m.Content, testOutput) {
			found = true
			break
		}
	}
	assert.True(t, found, "test failure output should be injected as user message")
}

func TestGetLastTaskFile_ExcludesHandoffAndChangePlan(t *testing.T) {
	dir := t.TempDir()

	// Create various files in the task directory
	files := []string{
		"001-add-retry-task.md",
		"001-add-retry-task-handoff.md",
		"001-add-retry-task-change-plan.md",
		"002-fix-bug-task.md",
	}
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("content"), 0o644))
	}

	result := getLastTaskFile(dir)
	assert.Equal(t, filepath.Join(dir, "002-fix-bug-task.md"), result)
}

func TestGetLastTaskFile_ExcludesEnrichedFiles(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"001-add-retry-task.md",
		"001-add-retry-task-enriched.md",
		"002-fix-bug-task.md",
		"002-fix-bug-task-enriched.md",
		"002-fix-bug-task-change-plan.md",
	}
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("content"), 0o644))
	}
	result := getLastTaskFile(dir)
	assert.Equal(t, filepath.Join(dir, "002-fix-bug-task.md"), result)
}

func TestGetLastTaskFile_OnlyEnrichedFiles(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"001-add-retry-task-enriched.md",
		"002-fix-bug-task-enriched.md",
	}
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("content"), 0o644))
	}
	result := getLastTaskFile(dir)
	assert.Equal(t, "", result)
}

func TestGetLastTaskFile_OnlyChangePlanFiles(t *testing.T) {
	dir := t.TempDir()

	// Only change-plan and handoff files -- should return empty
	files := []string{
		"001-add-retry-task-handoff.md",
		"001-add-retry-task-change-plan.md",
	}
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("content"), 0o644))
	}

	result := getLastTaskFile(dir)
	assert.Equal(t, "", result)
}

func TestGetLastTaskFile_ExcludesTestResultsFiles(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"001-add-retry-task.md",
		"001-add-retry-task-tests-iter1.md",
		"001-add-retry-task-tests-iter2.md",
	}
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("content"), 0o644))
	}

	result := getLastTaskFile(dir)
	assert.Equal(t, filepath.Join(dir, "001-add-retry-task.md"), result)
}

func TestPipeline_SwitchAgentClearsHistory(t *testing.T) {
	// Verify that SwitchAgent is called at all 4 agent-to-agent transitions (2, 3, 4, 5)
	// and that each call clears the conversation history (only system prompt + user msg).
	cwd := t.TempDir()
	taskDir := filepath.Join(cwd, ".fakeoid", "tasks")

	// Track the first message count seen at each agent's first LLM call.
	// If SwitchAgent cleared history, the first call for each agent should have
	// only system prompt + user message = 2 messages.
	type agentCall struct {
		systemPromptContains string
		msgCount             int
	}
	var agentCalls []agentCall

	callCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			callCount++
			switch callCount {
			case 1:
				// Agent 1 conversation
				onToken("I understand.")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case 2:
				// Agent 1 generate task
				onToken("# Task\n\nDo it.")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case 3:
				// Agent 2 (Prompt Engineer) -- first call after SwitchAgent
				agentCalls = append(agentCalls, agentCall{
					systemPromptContains: "Prompt Engineer",
					msgCount:             len(msgs),
				})
				// Return enriched content
				onToken("# Enriched Task\n\nDo it with context.")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case 4:
				// Agent 3 (Software Architect) -- first call after SwitchAgent
				agentCalls = append(agentCalls, agentCall{
					systemPromptContains: "Architect",
					msgCount:             len(msgs),
				})
				onToken("# Change Plan\n\n## Changes\n### CREATE main.go\n")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case 5:
				// Agent 4 (Software Engineer) -- first call after SwitchAgent
				agentCalls = append(agentCalls, agentCall{
					systemPromptContains: "Software Engineer",
					msgCount:             len(msgs),
				})
				onToken("```go:main.go\npackage main\n\nfunc Add(a, b int) int { return a + b }\n```\n")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case 6:
				// Agent 5 (QE Engineer) -- first call after SwitchAgent
				agentCalls = append(agentCalls, agentCall{
					systemPromptContains: "QE",
					msgCount:             len(msgs),
				})
				onToken("```go:main_test.go\npackage main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1,2) != 3 { t.Fatal(\"wrong\") }\n}\n```\n")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case 7:
				// Agent 6 (Course Corrector) -- first call after SwitchAgent
				agentCalls = append(agentCalls, agentCall{
					systemPromptContains: "Course Corrector",
					msgCount:             len(msgs),
				})
				onToken("## APPROVED\n\nWork matches the plan.")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			case 8:
				// Back to Agent 1
				onToken("Ready.")
				return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
			}
			return server.StreamResult{}, nil
		},
	}

	runner := agent.NewAgentRunner(taskDir)
	a1 := agent.NewAgent1(cwd, taskDir)
	runner.ActivateAgent(a1)

	require.NoError(t, os.WriteFile(filepath.Join(cwd, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	sh, _, _ := newTestShellWithRunner(t, "build a thing\n\ngo\n\ngo\n\nhello\n", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	defer sh.Close()

	err = sh.Run(context.Background())
	assert.NoError(t, err)

	// SwitchAgent must be called for Agent 2, 3, 4, 5, and 6 (5 transitions)
	assert.GreaterOrEqual(t, len(agentCalls), 5,
		"SwitchAgent must be called for Agent 2, 3, 4, 5, and 6 transitions")

	// Each agent's first call should have a fresh history (system prompt + user message = 2 msgs)
	for i, ac := range agentCalls {
		assert.Equal(t, 2, ac.msgCount,
			"Agent transition %d should have cleared history (expected 2 msgs: system+user, got %d)", i+1, ac.msgCount)
	}
}

func TestPrintHistoryDetail_FullPipeline(t *testing.T) {
	var buf bytes.Buffer
	fm := state.TaskFrontmatter{
		Timestamp:   time.Date(2026, 3, 18, 14, 30, 0, 0, time.UTC),
		SessionID:   "20260318-143000",
		Outcome:     "success",
		DurationSec: 45.2,
		TestResult:  "pass",
		Agents: []state.AgentOutcome{
			{Number: 1, Name: "Systems Engineer", Status: "success"},
			{Number: 2, Name: "Prompt Engineer", Status: "success"},
			{Number: 3, Name: "Software Architect", Status: "failed"},
			{Number: 4, Name: "Software Engineer", Status: "success"},
			{Number: 5, Name: "QE Engineer", Status: "skipped"},
		},
	}
	PrintHistoryDetail(&buf, "implement-auth", fm)
	out := buf.String()
	assert.Contains(t, out, "Task: implement-auth")
	assert.Contains(t, out, "Outcome: success")
	assert.Contains(t, out, "Duration: 45.2s")
	assert.Contains(t, out, "Test result: pass")
	assert.Contains(t, out, "Agent outcomes:")
	assert.Contains(t, out, "Systems Engineer")
	assert.Contains(t, out, "\u2713 success")
	assert.Contains(t, out, "X failed")
	assert.Contains(t, out, "- skipped")
}

func TestPrintHistoryDetail_NoAgents(t *testing.T) {
	var buf bytes.Buffer
	fm := state.TaskFrontmatter{
		Timestamp: time.Date(2026, 3, 18, 14, 30, 0, 0, time.UTC),
		Outcome:   "success",
	}
	PrintHistoryDetail(&buf, "old-task", fm)
	out := buf.String()
	assert.Contains(t, out, "No per-agent data available")
	assert.NotContains(t, out, "Agent outcomes:")
}

func TestHistoryDetail_ShowsAgentOutcomes(t *testing.T) {
	// Set up a temp state directory with history.json and a task file
	stateDir := t.TempDir()
	taskDir := filepath.Join(stateDir, "tasks")
	require.NoError(t, os.MkdirAll(taskDir, 0o755))

	// Create a task file with frontmatter containing AgentOutcome data
	fm := state.TaskFrontmatter{
		Timestamp:   time.Date(2026, 3, 18, 14, 30, 0, 0, time.UTC),
		SessionID:   "20260318-143000",
		Outcome:     "success",
		DurationSec: 12.5,
		Agents: []state.AgentOutcome{
			{Number: 1, Name: "Systems Engineer", Status: "success"},
			{Number: 2, Name: "Prompt Engineer", Status: "success"},
			{Number: 3, Name: "Software Architect", Status: "skipped"},
			{Number: 4, Name: "Software Engineer", Status: "success"},
			{Number: 5, Name: "QE Engineer", Status: "failed"},
		},
	}
	taskContent, err := state.InjectFrontmatter(fm, "# Task\n\nBuild something.")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "task-001.md"), []byte(taskContent), 0o644))

	// Create history.json pointing to the task file
	histIdx := state.HistoryIndex{
		Records: []state.HistoryRecord{
			{
				SessionID: "20260318-143000",
				Timestamp: time.Date(2026, 3, 18, 14, 30, 0, 0, time.UTC),
				TaskName:  "task-001",
				Outcome:   "success",
				TaskFile:  "tasks/task-001.md",
			},
		},
	}
	histData, err := json.Marshal(histIdx)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "history.json"), histData, 0o644))

	// Create a shell with stateDir set
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			return server.StreamResult{}, nil
		},
	}
	out := &bytes.Buffer{}
	sh, err := New(client, 8192, "test-model", "test-gpu", "",
		WithStdin(io.NopCloser(strings.NewReader(""))),
		WithStderr(out),
		WithStateDir(stateDir),
	)
	require.NoError(t, err)
	defer sh.Close()

	// Call historyDetail
	sh.historyDetail(1)

	output := out.String()
	assert.Contains(t, output, "Task: task-001")
	assert.Contains(t, output, "Outcome: success")
	assert.Contains(t, output, "Systems Engineer")
	assert.Contains(t, output, "QE Engineer")
	assert.Contains(t, output, "X failed")
	assert.Contains(t, output, "- skipped")
}

func TestHistoryDetail_InvalidIndex(t *testing.T) {
	stateDir := t.TempDir()

	// Create empty history
	histIdx := state.HistoryIndex{
		Records: []state.HistoryRecord{
			{SessionID: "1", TaskName: "task-001", Outcome: "success", TaskFile: "tasks/task-001.md"},
		},
	}
	histData, _ := json.Marshal(histIdx)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "history.json"), histData, 0o644))

	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			return server.StreamResult{}, nil
		},
	}
	out := &bytes.Buffer{}
	sh, err := New(client, 8192, "test-model", "test-gpu", "",
		WithStdin(io.NopCloser(strings.NewReader(""))),
		WithStderr(out),
		WithStateDir(stateDir),
	)
	require.NoError(t, err)
	defer sh.Close()

	sh.historyDetail(5)
	assert.Contains(t, out.String(), "Invalid task number: 5")
}

func TestHistoryDetail_NoStateDir(t *testing.T) {
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			return server.StreamResult{}, nil
		},
	}
	out := &bytes.Buffer{}
	sh, err := New(client, 8192, "test-model", "test-gpu", "",
		WithStdin(io.NopCloser(strings.NewReader(""))),
		WithStderr(out),
	)
	require.NoError(t, err)
	defer sh.Close()

	sh.historyDetail(1)
	assert.Contains(t, out.String(), "No history available")
}

func TestShell_WithAgentRunner_UsesRunnerHistory(t *testing.T) {
	taskDir := filepath.Join(t.TempDir(), ".fakeoid", "tasks")

	var capturedMsgs []server.Message
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			capturedMsgs = msgs
			onToken("response")
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	runner := agent.NewAgentRunner(taskDir)
	a1 := agent.NewAgent1(t.TempDir(), taskDir)
	runner.ActivateAgent(a1)

	sh, _, _ := newTestShellWithRunner(t, "hello\n", client, 8192, runner)
	defer sh.Close()

	err := sh.Run(context.Background())
	assert.NoError(t, err)

	// Messages should include system prompt from runner + user message
	require.NotEmpty(t, capturedMsgs)
	assert.Equal(t, "system", capturedMsgs[0].Role)
	assert.Contains(t, capturedMsgs[0].Content, "Systems Engineer")
	assert.Equal(t, "user", capturedMsgs[1].Role)
	assert.Equal(t, "hello", capturedMsgs[1].Content)
}

func TestResumeLastTask_FromHandoff(t *testing.T) {
	// When a handoff file exists, resume should start at Agent 5 (feedback loop).
	cwd := t.TempDir()
	stateDir := filepath.Join(cwd, ".fakeoid")
	taskDir := filepath.Join(stateDir, "tasks")
	require.NoError(t, os.MkdirAll(taskDir, 0o755))

	// Set up go.mod and source file
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "main.go"), []byte("package main\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644))

	// Write task file
	taskFile := filepath.Join(taskDir, "001-add-feature.md")
	require.NoError(t, os.WriteFile(taskFile, []byte("# Task: Add feature\n\n## Description\nAdd it."), 0o644))

	// Write handoff (indicates Agent 4 completed)
	handoffFile := filepath.Join(taskDir, "001-add-feature-handoff.md")
	require.NoError(t, os.WriteFile(handoffFile, []byte("# Handoff\n\n## Files Modified/Created\n\n- main.go (created)\n"), 0o644))

	// Write history with interrupted status
	histIdx := state.HistoryIndex{
		Records: []state.HistoryRecord{
			{SessionID: "20260320-100000", TaskName: "001-add-feature", Outcome: "interrupted", TaskFile: "tasks/001-add-feature.md"},
		},
	}
	histData, _ := json.Marshal(histIdx)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "history.json"), histData, 0o644))

	resumeCallCount := 0
	client := &mockClient{
		streamFn: func(ctx context.Context, msgs []server.Message, onToken func(string)) (server.StreamResult, error) {
			resumeCallCount++
			sysPrompt := ""
			if len(msgs) > 0 && msgs[0].Role == "system" {
				sysPrompt = msgs[0].Content
			}
			if strings.Contains(sysPrompt, "Course Corrector") {
				// Agent 6: approve
				onToken("## APPROVED\n\nWork matches the plan.")
			} else {
				// Agent 5: produce a passing test
				onToken("```go:main_test.go\npackage main\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1,2) != 3 { t.Fatal(\"wrong\") }\n}\n```\n")
			}
			return server.StreamResult{Usage: server.Usage{TotalTokens: 100}}, nil
		},
	}

	sb, err := sandbox.New(cwd, nil)
	require.NoError(t, err)
	defer sb.Close()

	runner := agent.NewAgentRunner(taskDir)
	a1 := agent.NewAgent1(cwd, taskDir)
	runner.ActivateAgent(a1)

	// Type "resume" then EOF
	sh, _, stderr := newTestShellWithRunner(t, "resume\n", client, 8192, runner)
	sh.cwd = cwd
	sh.sandbox = sb
	sh.stateDir = stateDir
	defer sh.Close()

	err = sh.Run(context.Background())
	assert.NoError(t, err)

	stderrStr := stderr.String()
	assert.Contains(t, stderrStr, "Resuming from Agent 5")
	assert.Contains(t, stderrStr, "Pipeline Complete: PASS")
}

func TestDetectResumeStage(t *testing.T) {
	dir := t.TempDir()
	base := "001-task"

	// No artifacts → Agent 2
	assert.Equal(t, "Agent 2: Prompt Engineer", detectResumeStage(dir, base))

	// Enriched exists → Agent 3
	require.NoError(t, os.WriteFile(filepath.Join(dir, base+"-enriched.md"), []byte("x"), 0o644))
	assert.Equal(t, "Agent 3: Software Architect", detectResumeStage(dir, base))

	// Change plan exists → Agent 4
	require.NoError(t, os.WriteFile(filepath.Join(dir, base+"-change-plan.md"), []byte("x"), 0o644))
	assert.Equal(t, "Agent 4: Software Engineer", detectResumeStage(dir, base))

	// Handoff exists → Agent 5
	require.NoError(t, os.WriteFile(filepath.Join(dir, base+"-handoff.md"), []byte("x"), 0o644))
	assert.Equal(t, "Agent 5: QE Engineer", detectResumeStage(dir, base))
}
