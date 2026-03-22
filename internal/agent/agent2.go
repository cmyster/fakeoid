package agent

import "os"

// Agent2 implements the Agent interface for the Prompt Engineer role.
// It transforms Agent 1's raw task prompt into a structured, context-enriched
// prompt by reading relevant source files and injecting their contents.
type Agent2 struct {
	cwd         string
	taskDir     string
	taskFile    string
	fileTree    string
	turnCount   int
	readFiles   map[string]string // path -> content
	tokenBudget int
	usedTokens  int
}

// NewAgent2 creates a new Agent 2 (Prompt Engineer) with the given working
// directory, task directory, task file path, and model context size.
// It scans the file tree at construction time and sets tokenBudget to 60%
// of ctxSize.
func NewAgent2(cwd, taskDir, taskFile string, ctxSize int) *Agent2 {
	tree, _ := ScanFileTree(cwd, 3, 0, nil)
	return &Agent2{
		cwd:         cwd,
		taskDir:     taskDir,
		taskFile:    taskFile,
		fileTree:    tree,
		readFiles:   make(map[string]string),
		tokenBudget: ctxSize * 60 / 100,
	}
}

// Number returns the agent's pipeline position.
func (a *Agent2) Number() int { return 2 }

// Name returns the human-readable agent name.
func (a *Agent2) Name() string { return "Prompt Engineer" }

// SystemPrompt returns the system prompt for Agent 2, including CWD, file tree,
// and the content of the task prompt file from Agent 1.
func (a *Agent2) SystemPrompt() string {
	taskContent := ""
	if a.taskFile != "" {
		data, err := os.ReadFile(a.taskFile)
		if err == nil {
			taskContent = string(data)
		}
	}
	return Agent2SystemPrompt(a.cwd, a.fileTree, taskContent)
}

// HandleResponse processes an LLM response. It increments the turn counter
// and checks for ```read blocks. Returns ActionContinue if read blocks are
// found and turnCount is under maxTurns; otherwise returns ActionComplete.
func (a *Agent2) HandleResponse(response string) Action {
	a.turnCount++
	paths := ParseReadBlocks(response)
	if len(paths) > 0 {
		return Action{Type: ActionContinue}
	}
	return Action{Type: ActionComplete}
}

// ReadFiles returns the accumulated read files map (path -> content).
func (a *Agent2) ReadFiles() map[string]string {
	return a.readFiles
}

// AddReadFile stores a file's content and updates the used token count.
func (a *Agent2) AddReadFile(path, content string) {
	a.readFiles[path] = content
	a.usedTokens += EstimateTokens(content)
}

// TokenBudgetRemaining returns the remaining token budget after accounting
// for files already read. Clamps to 0.
func (a *Agent2) TokenBudgetRemaining() int {
	remaining := a.tokenBudget - a.usedTokens
	if remaining < 0 {
		return 0
	}
	return remaining
}

// TaskDir returns the configured task directory path.
func (a *Agent2) TaskDir() string {
	return a.taskDir
}
