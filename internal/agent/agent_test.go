package agent

import (
	"testing"

	"github.com/cmyster/fakeoid/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAgent implements Agent for testing.
type mockAgent struct {
	number      int
	name        string
	systemPrompt string
}

func (m *mockAgent) Number() int            { return m.number }
func (m *mockAgent) Name() string           { return m.name }
func (m *mockAgent) SystemPrompt() string   { return m.systemPrompt }
func (m *mockAgent) HandleResponse(r string) Action {
	return Action{Type: ActionContinue}
}

func TestAgentRunner_SystemPrompt(t *testing.T) {
	runner := NewAgentRunner("/tmp/test-tasks")
	agent := &mockAgent{number: 1, name: "Test Agent", systemPrompt: "You are a test agent."}

	runner.ActivateAgent(agent)

	hist := runner.History()
	require.Len(t, hist, 1)
	assert.Equal(t, "system", hist[0].Role)
	assert.Equal(t, "You are a test agent.", hist[0].Content)
}

func TestAgentRunner_SwitchAgent(t *testing.T) {
	runner := NewAgentRunner("/tmp/test-tasks")
	agent1 := &mockAgent{number: 1, name: "Agent One", systemPrompt: "Prompt one"}
	agent2 := &mockAgent{number: 4, name: "Agent Four", systemPrompt: "Prompt four"}

	runner.ActivateAgent(agent1)
	runner.AppendUserMessage("hello")
	runner.AppendAssistantMessage("hi there")
	assert.Len(t, runner.History(), 3) // system + user + assistant

	runner.SwitchAgent(agent2)

	hist := runner.History()
	require.Len(t, hist, 1)
	assert.Equal(t, "system", hist[0].Role)
	assert.Equal(t, "Prompt four", hist[0].Content)
	assert.Equal(t, agent2, runner.Active())
}

func TestAgentRunner_AppendUserMessage(t *testing.T) {
	runner := NewAgentRunner("/tmp/test-tasks")
	agent := &mockAgent{number: 1, name: "Test", systemPrompt: "sys"}

	runner.ActivateAgent(agent)
	runner.AppendUserMessage("hello world")

	hist := runner.History()
	require.Len(t, hist, 2)
	assert.Equal(t, "system", hist[0].Role)
	assert.Equal(t, server.Message{Role: "user", Content: "hello world"}, hist[1])
}

func TestAgentRunner_AppendAssistantMessage(t *testing.T) {
	runner := NewAgentRunner("/tmp/test-tasks")
	agent := &mockAgent{number: 1, name: "Test", systemPrompt: "sys"}

	runner.ActivateAgent(agent)
	runner.AppendUserMessage("hello")
	runner.AppendAssistantMessage("I am an assistant")

	hist := runner.History()
	require.Len(t, hist, 3)
	assert.Equal(t, server.Message{Role: "assistant", Content: "I am an assistant"}, hist[2])
}

func TestAgentRunner_History(t *testing.T) {
	runner := NewAgentRunner("/tmp/test-tasks")
	agent := &mockAgent{number: 1, name: "Test", systemPrompt: "sys"}

	runner.ActivateAgent(agent)
	runner.AppendUserMessage("msg")

	hist1 := runner.History()
	hist2 := runner.History()

	// Modifying the returned slice should not affect internal state
	hist1[0].Content = "modified"
	assert.Equal(t, "sys", hist2[0].Content)
	assert.Equal(t, "sys", runner.History()[0].Content)
}
