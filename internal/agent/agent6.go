package agent

import (
	"os"
	"strings"
)

// Agent6 implements the Agent interface for the Course Corrector role.
// It is a single-turn agent that reviews Agent 4/5 output against the
// enriched prompt and change plan, producing an APPROVED or CORRECTION
// NEEDED verdict.
type Agent6 struct {
	taskDir         string
	enrichedFile    string // path to NNN-slug-task-enriched.md
	changePlanFile  string // path to NNN-slug-task-change-plan.md
	testOutput      string // raw test output from Agent 5
	handoffFile     string // path to NNN-slug-task-handoff.md
	enrichedContent string
	planContent     string
	handoffContent  string
}

// NewAgent6 creates a new Agent 6 (Course Corrector). It reads the enriched
// prompt, change plan, and handoff files at construction time (same pattern
// as Agent3/Agent4 reading files in constructor).
func NewAgent6(taskDir, enrichedFile, changePlanFile, testOutput, handoffFile string) *Agent6 {
	a := &Agent6{
		taskDir:        taskDir,
		enrichedFile:   enrichedFile,
		changePlanFile: changePlanFile,
		testOutput:     testOutput,
		handoffFile:    handoffFile,
	}
	if enrichedFile != "" {
		data, err := os.ReadFile(enrichedFile)
		if err == nil {
			a.enrichedContent = string(data)
		}
	}
	if changePlanFile != "" {
		data, err := os.ReadFile(changePlanFile)
		if err == nil {
			a.planContent = string(data)
		}
	}
	if handoffFile != "" {
		data, err := os.ReadFile(handoffFile)
		if err == nil {
			a.handoffContent = string(data)
		}
	}
	return a
}

// Number returns the agent's pipeline position.
func (a *Agent6) Number() int { return 6 }

// Name returns the human-readable agent name.
func (a *Agent6) Name() string { return "Course Corrector" }

// SystemPrompt returns the system prompt for Agent 6, including the enriched
// prompt, change plan, test output, and handoff summary.
func (a *Agent6) SystemPrompt() string {
	return Agent6SystemPrompt(a.enrichedContent, a.planContent, a.testOutput, a.handoffContent)
}

// HandleResponse processes an LLM response. Agent 6 is single-turn:
// it always returns ActionComplete after the first response.
func (a *Agent6) HandleResponse(response string) Action {
	return Action{Type: ActionComplete}
}

// ParseCorrectionVerdict detects whether the LLM response indicates approval
// or correction needed. If the response starts with "## APPROVED", it returns
// (true, ""). Otherwise it returns (false, response) -- treating unparseable
// responses as corrections needed.
func ParseCorrectionVerdict(response string) (approved bool, body string) {
	if strings.HasPrefix(strings.TrimSpace(response), "## APPROVED") {
		return true, ""
	}
	return false, response
}
