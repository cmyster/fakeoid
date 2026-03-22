package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent6_NewAgent6_ReturnsNonNil(t *testing.T) {
	dir := t.TempDir()
	enrichedFile := filepath.Join(dir, "001-test-enriched.md")
	changePlanFile := filepath.Join(dir, "001-test-change-plan.md")
	handoffFile := filepath.Join(dir, "001-test-handoff.md")

	require.NoError(t, os.WriteFile(enrichedFile, []byte("enriched content"), 0o644))
	require.NoError(t, os.WriteFile(changePlanFile, []byte("change plan content"), 0o644))
	require.NoError(t, os.WriteFile(handoffFile, []byte("handoff content"), 0o644))

	a := NewAgent6(dir, enrichedFile, changePlanFile, "test output here", handoffFile)
	require.NotNil(t, a)
	assert.Equal(t, 6, a.Number())
	assert.Equal(t, "Course Corrector", a.Name())
}

func TestAgent6_ImplementsAgent(t *testing.T) {
	dir := t.TempDir()
	enrichedFile := filepath.Join(dir, "001-test-enriched.md")
	changePlanFile := filepath.Join(dir, "001-test-change-plan.md")
	handoffFile := filepath.Join(dir, "001-test-handoff.md")

	require.NoError(t, os.WriteFile(enrichedFile, []byte("enriched"), 0o644))
	require.NoError(t, os.WriteFile(changePlanFile, []byte("plan"), 0o644))
	require.NoError(t, os.WriteFile(handoffFile, []byte("handoff"), 0o644))

	a := NewAgent6(dir, enrichedFile, changePlanFile, "test output", handoffFile)
	var _ Agent = a // compile-time interface check
}

func TestAgent6_HandleResponse_AlwaysComplete(t *testing.T) {
	a := &Agent6{}
	for i := 0; i < 5; i++ {
		action := a.HandleResponse("response")
		assert.Equal(t, ActionComplete, action.Type)
	}
}

func TestAgent6_SystemPrompt_ContainsAllSections(t *testing.T) {
	dir := t.TempDir()
	enrichedFile := filepath.Join(dir, "001-test-enriched.md")
	changePlanFile := filepath.Join(dir, "001-test-change-plan.md")
	handoffFile := filepath.Join(dir, "001-test-handoff.md")

	require.NoError(t, os.WriteFile(enrichedFile, []byte("ENRICHED_PROMPT_CONTENT"), 0o644))
	require.NoError(t, os.WriteFile(changePlanFile, []byte("CHANGE_PLAN_CONTENT"), 0o644))
	require.NoError(t, os.WriteFile(handoffFile, []byte("HANDOFF_CONTENT"), 0o644))

	a := NewAgent6(dir, enrichedFile, changePlanFile, "TEST_OUTPUT_CONTENT", handoffFile)
	prompt := a.SystemPrompt()

	assert.Contains(t, prompt, "ENRICHED_PROMPT_CONTENT")
	assert.Contains(t, prompt, "CHANGE_PLAN_CONTENT")
	assert.Contains(t, prompt, "TEST_OUTPUT_CONTENT")
	assert.Contains(t, prompt, "HANDOFF_CONTENT")
}

func TestAgent6SystemPrompt_ContainsFourSections(t *testing.T) {
	prompt := Agent6SystemPrompt("enriched", "changeplan", "testout", "handoff")
	assert.Contains(t, prompt, "enriched")
	assert.Contains(t, prompt, "changeplan")
	assert.Contains(t, prompt, "testout")
	assert.Contains(t, prompt, "handoff")
}

func TestParseCorrectionVerdict(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantApproved bool
		wantBody     string
	}{
		{
			name:         "approved simple",
			input:        "## APPROVED\n",
			wantApproved: true,
			wantBody:     "",
		},
		{
			name:         "approved with notes",
			input:        "## APPROVED\nsome notes here",
			wantApproved: true,
			wantBody:     "",
		},
		{
			name:         "approved with leading whitespace",
			input:        "  ## APPROVED\nsome notes",
			wantApproved: true,
			wantBody:     "",
		},
		{
			name:         "correction needed",
			input:        "## CORRECTION NEEDED\n### Plan Drift\nSome drift",
			wantApproved: false,
			wantBody:     "## CORRECTION NEEDED\n### Plan Drift\nSome drift",
		},
		{
			name:         "random garbage",
			input:        "random text here",
			wantApproved: false,
			wantBody:     "random text here",
		},
		{
			name:         "empty string",
			input:        "",
			wantApproved: false,
			wantBody:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			approved, body := ParseCorrectionVerdict(tt.input)
			assert.Equal(t, tt.wantApproved, approved)
			assert.Equal(t, tt.wantBody, body)
		})
	}
}
