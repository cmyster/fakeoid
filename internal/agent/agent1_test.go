package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent1_ImplementsAgent(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	var _ Agent = a // compile-time check
	assert.NotNil(t, a)
}

func TestAgent1_Number(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	assert.Equal(t, 1, a.Number())
}

func TestAgent1_Name(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	assert.Equal(t, "Systems Engineer", a.Name())
}

func TestAgent1_SystemPrompt(t *testing.T) {
	a := NewAgent1("/tmp/test-cwd", "/tmp/tasks")
	prompt := a.SystemPrompt()
	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "Systems Engineer")
	assert.Contains(t, prompt, "/tmp/test-cwd")
}

func TestAgent1_HandleResponse_Gathering(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	action := a.HandleResponse("Here is my response about your task")
	assert.Equal(t, ActionContinue, action.Type)
	// turnCount should increment
	assert.Equal(t, 1, a.turnCount)
}

func TestAgent1_TriggerGo(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	a.turnCount = 1 // simulate conversation happened
	assert.True(t, a.IsTrigger("go"))
}

func TestAgent1_TriggerDone(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	a.turnCount = 1
	assert.True(t, a.IsTrigger("done"))
}

func TestAgent1_TriggerGoCase(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	a.turnCount = 1
	assert.True(t, a.IsTrigger("Go"))
	assert.True(t, a.IsTrigger("GO"))
}

func TestAgent1_TriggerInSentence(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	a.turnCount = 1
	assert.False(t, a.IsTrigger("let's go ahead"))
}

func TestAgent1_TriggerWithWhitespace(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	a.turnCount = 1
	assert.True(t, a.IsTrigger("  go  "))
}

func TestAgent1_TriggerNoContent(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	// turnCount is 0 -- no conversation has happened
	assert.False(t, a.IsTrigger("go"))
}

func TestAgent1_StateTransition_GatherToGenerate(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	a.turnCount = 1 // simulate at least one exchange

	require.True(t, a.IsTrigger("go"))
	action := a.ProcessTrigger()

	assert.Equal(t, ActionGenerate, action.Type)
	assert.Equal(t, stateGenerating, a.State())
}

func TestAgent1_StateTransition_ConfirmToComplete(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	a.turnCount = 1
	a.SetConfirming()

	require.True(t, a.IsTrigger("done"))
	action := a.ProcessTrigger()

	assert.Equal(t, ActionComplete, action.Type)
}

func TestAgent1_StateTransition_ConfirmCorrection(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	a.SetConfirming()

	// Non-trigger input while confirming should go back to gathering
	assert.False(t, a.IsTrigger("actually, also add error handling"))
	a.SetGathering()

	assert.Equal(t, stateGathering, a.State())
}

func TestAgent1_SetConfirming(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	a.SetConfirming()
	assert.Equal(t, stateConfirming, a.State())
}

func TestAgent1_TaskDir(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/my-tasks")
	assert.Equal(t, "/tmp/my-tasks", a.TaskDir())
}

func TestAgent1_HandleResponse_DetectsTaskPrompt(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	response := `# Task: Create Rust Application

## Description
Create a Rust application that prints the kernel version.

## Affected Files
- Cargo.toml
- src/main.rs

## Expected Tests
- Unit test for kernel version function

## Acceptance Criteria
- Application compiles without errors
- Prints correct kernel version

## Context
N/A`
	action := a.HandleResponse(response)
	assert.Equal(t, ActionComplete, action.Type)
	assert.Equal(t, response, action.Output)
}

func TestAgent1_HandleResponse_ClarifyingQuestion(t *testing.T) {
	a := NewAgent1("/tmp/test", "/tmp/tasks")
	response := "I have a few questions about your task. What language should this be in?"
	action := a.HandleResponse(response)
	assert.Equal(t, ActionContinue, action.Type)
}
