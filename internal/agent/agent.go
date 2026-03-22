package agent

import (
	"github.com/cmyster/fakeoid/internal/server"
)

// Agent defines the behavior contract for pipeline agents.
type Agent interface {
	// Number returns the agent's pipeline position (1, 4, 5).
	Number() int
	// Name returns the human-readable agent name.
	Name() string
	// SystemPrompt returns the system prompt for this agent.
	SystemPrompt() string
	// HandleResponse processes an LLM response and returns the next action.
	HandleResponse(response string) Action
}

// ActionType represents what the runner should do next.
type ActionType int

const (
	// ActionContinue means keep conversing, wait for user input.
	ActionContinue ActionType = iota
	// ActionGenerate means the agent wants to generate its output.
	ActionGenerate
	// ActionComplete means the agent is done, ready for handoff.
	ActionComplete
)

// Action tells the runner what to do next after an agent processes a response.
type Action struct {
	Type   ActionType
	Output string // optional message to display to user
}

// AgentRunner manages the active agent, conversation history, and task directory.
// It does NOT hold a ChatClient -- the Shell calls its own client with runner.History().
type AgentRunner struct {
	active  Agent
	history []server.Message
	taskDir string
}

// NewAgentRunner creates a new AgentRunner with the given task directory.
func NewAgentRunner(taskDir string) *AgentRunner {
	return &AgentRunner{
		taskDir: taskDir,
	}
}

// ActivateAgent clears history and sets the given agent as active with its
// system prompt as the first message.
func (r *AgentRunner) ActivateAgent(a Agent) {
	r.history = []server.Message{
		{Role: "system", Content: a.SystemPrompt()},
	}
	r.active = a
}

// SwitchAgent clears history and activates a new agent.
func (r *AgentRunner) SwitchAgent(a Agent) {
	r.ActivateAgent(a)
}

// Active returns the currently active agent.
func (r *AgentRunner) Active() Agent {
	return r.active
}

// AppendUserMessage adds a user message to the conversation history.
func (r *AgentRunner) AppendUserMessage(content string) {
	r.history = append(r.history, server.Message{
		Role:    "user",
		Content: content,
	})
}

// AppendAssistantMessage adds an assistant message to the conversation history.
func (r *AgentRunner) AppendAssistantMessage(content string) {
	r.history = append(r.history, server.Message{
		Role:    "assistant",
		Content: content,
	})
}

// History returns a copy of the current conversation history.
func (r *AgentRunner) History() []server.Message {
	cp := make([]server.Message, len(r.history))
	copy(cp, r.history)
	return cp
}

// HistoryLen returns the number of messages in the conversation history.
func (r *AgentRunner) HistoryLen() int {
	return len(r.history)
}

// TrimOldest removes the two oldest non-system messages (one user+assistant pair)
// from the history. Returns the estimated token count of the dropped messages
// (using char/4 estimation). Returns 0 if there are fewer than 3 non-system messages.
func (r *AgentRunner) TrimOldest() int {
	// Keep system prompt (index 0) + at least one pair
	if len(r.history) < 4 {
		return 0
	}
	// Drop history[1] and history[2] (oldest user+assistant after system)
	dropped := len(r.history[1].Content)/4 + len(r.history[2].Content)/4
	r.history = append(r.history[:1], r.history[3:]...)
	return dropped
}

// TaskDir returns the configured task directory path.
func (r *AgentRunner) TaskDir() string {
	return r.taskDir
}
