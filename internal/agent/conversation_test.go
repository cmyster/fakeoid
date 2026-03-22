package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/cmyster/fakeoid/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatConversation_SingleTurn(t *testing.T) {
	messages := []server.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: "Hi there!"},
	}
	result := FormatConversation(messages)
	assert.Contains(t, result, "## System Prompt")
	assert.Contains(t, result, "You are a helpful assistant.")
	assert.Contains(t, result, "## Conversation")
	assert.Contains(t, result, "### User")
	assert.Contains(t, result, "Hello world")
	assert.Contains(t, result, "### Assistant")
	assert.Contains(t, result, "Hi there!")
	// Single-turn should NOT have "## Turn" headings
	assert.NotContains(t, result, "## Turn")
}

func TestFormatConversation_MultiTurn(t *testing.T) {
	messages := []server.Message{
		{Role: "system", Content: "System prompt here."},
		{Role: "user", Content: "First question"},
		{Role: "assistant", Content: "First answer"},
		{Role: "user", Content: "Second question"},
		{Role: "assistant", Content: "Second answer"},
	}
	result := FormatConversation(messages)
	assert.Contains(t, result, "## System Prompt")
	assert.Contains(t, result, "System prompt here.")
	assert.Contains(t, result, "## Turn 1")
	assert.Contains(t, result, "## Turn 2")
	assert.Contains(t, result, "First question")
	assert.Contains(t, result, "First answer")
	assert.Contains(t, result, "Second question")
	assert.Contains(t, result, "Second answer")
	// Multi-turn should NOT have "## Conversation" heading
	assert.NotContains(t, result, "## Conversation\n")
}

func TestFormatConversation_SystemOnly(t *testing.T) {
	messages := []server.Message{
		{Role: "system", Content: "Only system prompt."},
	}
	result := FormatConversation(messages)
	assert.Contains(t, result, "## System Prompt")
	assert.Contains(t, result, "Only system prompt.")
	assert.NotContains(t, result, "## Turn")
	assert.NotContains(t, result, "## Conversation")
}

func TestFormatConversation_Empty(t *testing.T) {
	result := FormatConversation(nil)
	assert.Equal(t, "", result)

	result2 := FormatConversation([]server.Message{})
	assert.Equal(t, "", result2)
}

func TestConversationFrontmatter_MarshalYAML(t *testing.T) {
	fm := ConversationFrontmatter{
		AgentNumber: 2,
		AgentName:   "Prompt Engineer",
		Timestamp:   time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC),
		DurationSec: 12.5,
	}
	result := marshalConversationFrontmatter(fm)
	assert.Contains(t, result, "agent_number: 2")
	assert.Contains(t, result, "agent_name: Prompt Engineer")
	assert.Contains(t, result, "duration_sec: 12.5")
	assert.Contains(t, result, "timestamp:")
}

func TestWriteConversationFile_NoIteration(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	messages := []server.Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "User input"},
		{Role: "assistant", Content: "Response"},
	}

	path, err := WriteConversationFile(sb, taskDir, "001-my-task.md", 2, "Prompt Engineer", 0, messages, 5*time.Second)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, "001-my-task-agent2-conversation.md"),
		"expected path ending in 001-my-task-agent2-conversation.md, got %s", path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.True(t, strings.HasPrefix(content, "---\n"), "file should start with YAML frontmatter")
	assert.Contains(t, content, "agent_number: 2")
	assert.Contains(t, content, "agent_name: Prompt Engineer")
}

func TestWriteConversationFile_WithIteration(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	messages := []server.Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Fix code"},
		{Role: "assistant", Content: "Fixed!"},
	}

	path, err := WriteConversationFile(sb, taskDir, "001-my-task.md", 4, "Code Writer", 2, messages, 10*time.Second)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, "001-my-task-agent4-iter2-conversation.md"),
		"expected path ending in -agent4-iter2-conversation.md, got %s", path)
}

func TestWriteConversationFile_Agent5WithIteration(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	messages := []server.Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Test code"},
		{Role: "assistant", Content: "Tests pass"},
	}

	path, err := WriteConversationFile(sb, taskDir, "001-my-task.md", 5, "Tester", 1, messages, 8*time.Second)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, "001-my-task-agent5-iter1-conversation.md"),
		"expected path ending in -agent5-iter1-conversation.md, got %s", path)
}

func TestWriteConversationFile_FrontmatterContent(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "tasks")

	sb, err := sandbox.New(dir, nil)
	require.NoError(t, err)
	defer sb.Close()

	messages := []server.Message{
		{Role: "system", Content: "You are Agent 1."},
		{Role: "user", Content: "Do something"},
		{Role: "assistant", Content: "Done"},
	}

	path, err := WriteConversationFile(sb, taskDir, "002-test.md", 1, "Systems Engineer", 0, messages, 3*time.Second)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)

	// Check YAML frontmatter block
	assert.True(t, strings.HasPrefix(content, "---\n"))
	assert.Contains(t, content, "agent_number: 1")
	assert.Contains(t, content, "agent_name: Systems Engineer")
	assert.Contains(t, content, "duration_sec:")

	// Check markdown body
	assert.Contains(t, content, "## System Prompt")
	assert.Contains(t, content, "You are Agent 1.")
}

func TestWriteConversationFile_NilSandbox(t *testing.T) {
	messages := []server.Message{
		{Role: "system", Content: "prompt"},
	}
	_, err := WriteConversationFile(nil, "/tmp/tasks", "001-test.md", 1, "Test", 0, messages, time.Second)
	assert.Error(t, err)
}
