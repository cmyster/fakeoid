package agent

import (
	"strings"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// Agent6 implements the Agent interface for the QA Tester role.
// Each instance handles one testing scope (blackbox or whitebox) as
// assigned by Agent 5 (QA Team Leader). Multiple Agent6 instances
// can run sequentially: 6.1 (blackbox) must pass before 6.2 (whitebox).
type Agent6 struct {
	cwd            string
	taskDir        string
	scope          TestScope
	plan           TestPlanEntry
	fileTree       string
	readmeContent  string
	turnCount      int
	sb             *sandbox.Sandbox
}

// NewAgent6BlackBox creates an Agent 6 instance for black-box sanity/smoke testing.
// It receives the task purpose and build/run commands but NOT source code.
// If the test plan entry has no build command, it falls back to the README.
func NewAgent6BlackBox(cwd, taskDir string, plan TestPlanEntry, readmeContent string, sb *sandbox.Sandbox) *Agent6 {
	tree, _ := ScanFileTree(cwd, 3, 0, nil)
	return &Agent6{
		cwd:           cwd,
		taskDir:       taskDir,
		scope:         ScopeBlackBox,
		plan:          plan,
		fileTree:      tree,
		readmeContent: readmeContent,
		sb:            sb,
	}
}

// NewAgent6WhiteBox creates an Agent 6 instance for white-box unit/integration testing.
// It receives the source code but NOT the task intent/purpose.
func NewAgent6WhiteBox(cwd, taskDir string, plan TestPlanEntry, sb *sandbox.Sandbox) *Agent6 {
	tree, _ := ScanFileTree(cwd, 3, 0, nil)

	// Read source files for the whitebox scope
	sourceCode := readSourceFilesWithSandbox(cwd, plan.SourceFiles, sb, 16384)
	plan.SourceCode = sourceCode

	return &Agent6{
		cwd:     cwd,
		taskDir: taskDir,
		scope:   ScopeWhiteBox,
		plan:    plan,
		fileTree: tree,
		sb:      sb,
	}
}

// Number returns the agent's pipeline position.
func (a *Agent6) Number() int { return 6 }

// Name returns the human-readable agent name with scope suffix.
func (a *Agent6) Name() string {
	switch a.scope {
	case ScopeBlackBox:
		return "QA Tester (Black-Box)"
	case ScopeWhiteBox:
		return "QA Tester (White-Box)"
	default:
		return "QA Tester"
	}
}

// Scope returns the testing scope of this Agent 6 instance.
func (a *Agent6) Scope() TestScope {
	return a.scope
}

// SystemPrompt returns the system prompt for Agent 6, tailored to its scope.
func (a *Agent6) SystemPrompt() string {
	switch a.scope {
	case ScopeBlackBox:
		return Agent6BlackBoxSystemPrompt(a.cwd, a.fileTree, a.plan.Purpose, a.plan.BuildCmd, a.plan.RunCmd, a.plan.Tests, a.readmeContent)
	case ScopeWhiteBox:
		return Agent6WhiteBoxSystemPrompt(a.cwd, a.fileTree, a.plan.SourceCode, a.plan.SourceFiles, a.plan.Tests)
	default:
		return ""
	}
}

// HandleResponse processes an LLM response. It increments the turn counter and
// checks for code blocks. Returns ActionComplete if code blocks are found;
// otherwise returns ActionContinue for multi-turn conversation.
func (a *Agent6) HandleResponse(response string) Action {
	a.turnCount++
	blocks := ParseCodeBlocks(response)
	if len(blocks) > 0 {
		return Action{Type: ActionComplete}
	}
	return Action{Type: ActionContinue}
}

// Agent6FailureReport represents a structured failure from a QA Tester sub-agent.
type Agent6FailureReport struct {
	Scope        TestScope // which sub-agent failed
	TestName     string    // which test failed
	Expected     string    // expected result (whitebox)
	Actual       string    // actual result (whitebox)
	Location     string    // file:line location (whitebox)
	TestOutput   string    // raw test output
	IsFatal      bool      // true for blackbox failures (no point continuing)
}

// FormatForAgent4 formats the failure report for Agent 4 to act on.
func (r *Agent6FailureReport) FormatForAgent4() string {
	var b strings.Builder
	if r.Scope == ScopeBlackBox {
		b.WriteString("## Black-Box Test Failure (FATAL)\n\n")
		b.WriteString("The code is fundamentally broken -- it fails basic sanity/smoke testing.\n")
		b.WriteString("Agent 4 must fix the code before any further testing is possible.\n\n")
		if r.TestName != "" {
			b.WriteString("### Failed Test\n")
			b.WriteString(r.TestName + "\n\n")
		}
		b.WriteString("### Test Output\n```\n")
		b.WriteString(r.TestOutput)
		b.WriteString("\n```\n")
	} else {
		b.WriteString("## White-Box Test Failure\n\n")
		if r.TestName != "" {
			b.WriteString("### Failed Test\n")
			b.WriteString(r.TestName + "\n\n")
		}
		if r.Location != "" {
			b.WriteString("### Location\n")
			b.WriteString(r.Location + "\n\n")
		}
		if r.Expected != "" {
			b.WriteString("### Expected\n")
			b.WriteString(r.Expected + "\n\n")
		}
		if r.Actual != "" {
			b.WriteString("### Actual\n")
			b.WriteString(r.Actual + "\n\n")
		}
		b.WriteString("### Test Output\n```\n")
		b.WriteString(r.TestOutput)
		b.WriteString("\n```\n")
	}
	return b.String()
}

// ReadSourceForWhiteBox reads the source files specified in a test plan entry
// and populates the SourceCode field. Used by the shell to prepare whitebox context.
func ReadSourceForWhiteBox(cwd string, entry *TestPlanEntry, sb *sandbox.Sandbox) {
	entry.SourceCode = readSourceFilesWithSandbox(cwd, entry.SourceFiles, sb, 16384)
}
