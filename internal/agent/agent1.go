package agent

import "strings"

// agent1State represents the state of Agent 1's conversation state machine.
type agent1State int

const (
	stateGathering  agent1State = iota // Free conversation, collecting task details
	stateGenerating                    // Trigger detected, task prompt being generated
	stateConfirming                    // Task file written, awaiting user confirmation
)

// Agent1 implements the Agent interface for the Systems Engineer role.
// It uses a state machine driven by user input (not LLM output) to manage
// the flow: gathering -> generating -> confirming -> complete.
type Agent1 struct {
	state     agent1State
	cwd       string
	taskDir   string
	fileTree  string
	turnCount int // number of LLM response turns (for trigger guard)
}

// NewAgent1 creates a new Agent 1 (Systems Engineer) with the given working
// directory and task output directory. It scans the file tree at construction
// time (static per session).
func NewAgent1(cwd, taskDir string) *Agent1 {
	tree, _ := ScanFileTree(cwd, 3, 0, nil)
	return &Agent1{
		state:   stateGathering,
		cwd:     cwd,
		taskDir: taskDir,
		fileTree: tree,
	}
}

// Number returns the agent's pipeline position.
func (a *Agent1) Number() int { return 1 }

// Name returns the human-readable agent name.
func (a *Agent1) Name() string { return "Systems Engineer" }

// SystemPrompt returns the system prompt for Agent 1, including CWD and file tree.
func (a *Agent1) SystemPrompt() string {
	return Agent1SystemPrompt(a.cwd, a.fileTree)
}

// HandleResponse processes an LLM response. It increments the turn counter
// and checks whether the response contains a complete task prompt. If a
// complete task prompt is detected, it returns ActionComplete so the shell
// can auto-proceed to the pipeline without requiring manual "go" triggers.
func (a *Agent1) HandleResponse(response string) Action {
	a.turnCount++
	if isCompleteTaskPrompt(response) {
		return Action{Type: ActionComplete, Output: response}
	}
	return Action{Type: ActionContinue}
}

// isCompleteTaskPrompt checks whether a response contains the structured task
// prompt format with the required section headers.
func isCompleteTaskPrompt(response string) bool {
	lower := strings.ToLower(response)
	return strings.Contains(lower, "# task:") &&
		strings.Contains(lower, "## description") &&
		strings.Contains(lower, "## acceptance criteria")
}

// IsTrigger returns true if the input is a trigger word ("go" or "done"),
// case-insensitive, matching only the entire trimmed input. Returns false
// if no conversation exchanges have occurred yet (turnCount < 1).
func (a *Agent1) IsTrigger(input string) bool {
	if a.turnCount < 1 {
		return false
	}
	trimmed := strings.ToLower(strings.TrimSpace(input))
	return trimmed == "go" || trimmed == "done"
}

// ProcessTrigger advances the state machine when a trigger word is detected.
// Must only be called when IsTrigger returns true.
//   - gathering -> generating: returns ActionGenerate
//   - confirming -> complete: returns ActionComplete
//   - otherwise: returns ActionContinue
func (a *Agent1) ProcessTrigger() Action {
	switch a.state {
	case stateGathering:
		a.state = stateGenerating
		return Action{Type: ActionGenerate}
	case stateConfirming:
		return Action{Type: ActionComplete}
	default:
		return Action{Type: ActionContinue}
	}
}

// SetConfirming transitions to the confirming state. Called by the shell
// after writing the task file and showing the summary to the user.
func (a *Agent1) SetConfirming() {
	a.state = stateConfirming
}

// SetGathering transitions back to the gathering state. Called when the user
// provides corrections after seeing the summary.
func (a *Agent1) SetGathering() {
	a.state = stateGathering
}

// TaskDir returns the configured task directory path.
func (a *Agent1) TaskDir() string {
	return a.taskDir
}

// State returns the current state (exported for testing).
func (a *Agent1) State() agent1State {
	return a.state
}

// IsConfirming returns true if the agent is in the confirming state.
func (a *Agent1) IsConfirming() bool {
	return a.state == stateConfirming
}

// IsGathering returns true if the agent is in the initial task-gathering state.
func (a *Agent1) IsGathering() bool {
	return a.state == stateGathering
}
