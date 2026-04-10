package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent6BlackBox_ImplementsAgent(t *testing.T) {
	dir := t.TempDir()
	plan := TestPlanEntry{
		Scope:    ScopeBlackBox,
		Purpose:  "A CLI tool",
		BuildCmd: "go build ./...",
		RunCmd:   "./tool",
		Tests:    []string{"Build succeeds"},
	}
	a := NewAgent6BlackBox(dir, dir, plan, "", nil)
	var _ Agent = a // compile-time interface check
	assert.NotNil(t, a)
}

func TestAgent6WhiteBox_ImplementsAgent(t *testing.T) {
	dir := t.TempDir()
	plan := TestPlanEntry{
		Scope:       ScopeWhiteBox,
		SourceFiles: []string{"internal/foo.go"},
		Tests:       []string{"Test Foo returns bar"},
	}
	a := NewAgent6WhiteBox(dir, dir, plan, nil)
	var _ Agent = a
	assert.NotNil(t, a)
}

func TestAgent6_Number(t *testing.T) {
	a := &Agent6{}
	assert.Equal(t, 6, a.Number())
}

func TestAgent6BlackBox_Name(t *testing.T) {
	a := &Agent6{scope: ScopeBlackBox}
	assert.Equal(t, "QA Tester (Black-Box)", a.Name())
}

func TestAgent6WhiteBox_Name(t *testing.T) {
	a := &Agent6{scope: ScopeWhiteBox}
	assert.Equal(t, "QA Tester (White-Box)", a.Name())
}

func TestAgent6_Scope(t *testing.T) {
	a := &Agent6{scope: ScopeBlackBox}
	assert.Equal(t, ScopeBlackBox, a.Scope())

	a2 := &Agent6{scope: ScopeWhiteBox}
	assert.Equal(t, ScopeWhiteBox, a2.Scope())
}

func TestAgent6_HandleResponse_WithCodeBlocks(t *testing.T) {
	a := &Agent6{}
	response := "Here is the test:\n```bash:test.sh\n#!/bin/bash\necho ok\n```"
	action := a.HandleResponse(response)
	assert.Equal(t, ActionComplete, action.Type)
}

func TestAgent6_HandleResponse_NoCodeBlocks(t *testing.T) {
	a := &Agent6{}
	response := "I need more context to write the tests."
	action := a.HandleResponse(response)
	assert.Equal(t, ActionContinue, action.Type)
	assert.Equal(t, 1, a.turnCount)
}

func TestAgent6BlackBox_SystemPrompt_ContainsPurpose(t *testing.T) {
	dir := t.TempDir()
	plan := TestPlanEntry{
		Scope:    ScopeBlackBox,
		Purpose:  "A tool that lists files",
		BuildCmd: "go build ./...",
		RunCmd:   "./fakeoid /tmp",
		Tests:    []string{"Build succeeds", "Lists files correctly"},
	}
	a := NewAgent6BlackBox(dir, dir, plan, "# Project\n## Build\ngo build ./...\n", nil)
	prompt := a.SystemPrompt()

	assert.Contains(t, prompt, "Black-Box")
	assert.Contains(t, prompt, "A tool that lists files")
	assert.Contains(t, prompt, "go build ./...")
	assert.Contains(t, prompt, "./fakeoid /tmp")
	assert.Contains(t, prompt, "Build succeeds")
	assert.Contains(t, prompt, "Lists files correctly")
	// Should NOT contain source code references
	assert.NotContains(t, prompt, "package agent")
}

func TestAgent6WhiteBox_SystemPrompt_ContainsCode(t *testing.T) {
	plan := TestPlanEntry{
		Scope:       ScopeWhiteBox,
		SourceFiles: []string{"internal/foo.go"},
		SourceCode:  "### internal/foo.go\n```go\npackage foo\n\nfunc Bar() {}\n```\n",
		Tests:       []string{"Test Bar returns nil"},
	}
	dir := t.TempDir()
	a := &Agent6{
		cwd:     dir,
		taskDir: dir,
		scope:   ScopeWhiteBox,
		plan:    plan,
	}
	prompt := a.SystemPrompt()

	assert.Contains(t, prompt, "White-Box")
	assert.Contains(t, prompt, "internal/foo.go")
	assert.Contains(t, prompt, "package foo")
	assert.Contains(t, prompt, "Test Bar returns nil")
}

// --- Agent6FailureReport tests ---

func TestAgent6FailureReport_BlackBox(t *testing.T) {
	r := &Agent6FailureReport{
		Scope:      ScopeBlackBox,
		TestName:   "Sanity: build",
		TestOutput: "exit code 1: compilation failed",
		IsFatal:    true,
	}
	formatted := r.FormatForAgent4()
	assert.Contains(t, formatted, "Black-Box Test Failure (FATAL)")
	assert.Contains(t, formatted, "fundamentally broken")
	assert.Contains(t, formatted, "Sanity: build")
	assert.Contains(t, formatted, "compilation failed")
}

func TestAgent6FailureReport_WhiteBox(t *testing.T) {
	r := &Agent6FailureReport{
		Scope:      ScopeWhiteBox,
		TestName:   "TestParseConfig",
		Expected:   "error",
		Actual:     "nil",
		Location:   "internal/config_test.go:42",
		TestOutput: "FAIL: TestParseConfig expected error got nil",
	}
	formatted := r.FormatForAgent4()
	assert.Contains(t, formatted, "White-Box Test Failure")
	assert.Contains(t, formatted, "TestParseConfig")
	assert.Contains(t, formatted, "internal/config_test.go:42")
	assert.Contains(t, formatted, "error")
	assert.Contains(t, formatted, "nil")
	assert.NotContains(t, formatted, "FATAL")
}

// --- Agent6 prompt function tests ---

func TestAgent6BlackBoxSystemPrompt_ContainsAllSections(t *testing.T) {
	prompt := Agent6BlackBoxSystemPrompt("/tmp/test", "tree", "A CLI tool", "make", "./run", []string{"Build passes", "Runs OK"}, "# README")
	assert.Contains(t, prompt, "A CLI tool")
	assert.Contains(t, prompt, "make")
	assert.Contains(t, prompt, "./run")
	assert.Contains(t, prompt, "Build passes")
	assert.Contains(t, prompt, "Runs OK")
	assert.Contains(t, prompt, "# README")
}

func TestAgent6WhiteBoxSystemPrompt_ContainsAllSections(t *testing.T) {
	prompt := Agent6WhiteBoxSystemPrompt("/tmp/test", "tree", "package foo", []string{"foo.go"}, []string{"Test Foo"})
	assert.Contains(t, prompt, "package foo")
	assert.Contains(t, prompt, "foo.go")
	assert.Contains(t, prompt, "Test Foo")
}

// --- TestScope constants ---

func TestTestScope_Values(t *testing.T) {
	require.Equal(t, TestScope("blackbox"), ScopeBlackBox)
	require.Equal(t, TestScope("whitebox"), ScopeWhiteBox)
}
