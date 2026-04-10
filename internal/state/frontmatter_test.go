package state

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjectFrontmatter(t *testing.T) {
	fm := TaskFrontmatter{
		Timestamp:   time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC),
		SessionID:   "20260316-120000",
		Outcome:     "success",
		Agents:      []AgentOutcome{{Number: 1, Name: "Systems Engineer", Status: "success"}, {Number: 4, Name: "Software Engineer", Status: "success"}, {Number: 5, Name: "QA Team Leader", Status: "success"}},
		DurationSec: 42.5,
	}
	result, err := InjectFrontmatter(fm, "# Task")
	require.NoError(t, err)
	assert.Contains(t, result, "---\n")
	assert.Contains(t, result, "session_id: 20260316-120000")
	assert.Contains(t, result, "outcome: success")
	assert.True(t, len(result) > len("# Task"))
	// Must end with the original content
	assert.Contains(t, result, "# Task")
}

func TestParseFrontmatter(t *testing.T) {
	fm := TaskFrontmatter{
		Timestamp:   time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC),
		SessionID:   "20260316-120000",
		Outcome:     "success",
		Agents:      []AgentOutcome{{Number: 1, Name: "Systems Engineer", Status: "success"}, {Number: 4, Name: "Software Engineer", Status: "success"}, {Number: 5, Name: "QA Team Leader", Status: "success"}},
		DurationSec: 42.5,
	}
	injected, err := InjectFrontmatter(fm, "# Task\nSome content")
	require.NoError(t, err)

	parsed, body, err := ParseFrontmatter(injected)
	require.NoError(t, err)
	assert.Equal(t, "20260316-120000", parsed.SessionID)
	assert.Equal(t, "success", parsed.Outcome)
	require.Len(t, parsed.Agents, 3)
	assert.Equal(t, 1, parsed.Agents[0].Number)
	assert.Equal(t, "Systems Engineer", parsed.Agents[0].Name)
	assert.Equal(t, "success", parsed.Agents[0].Status)
	assert.InDelta(t, 42.5, parsed.DurationSec, 0.01)
	assert.Equal(t, "# Task\nSome content", body)
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Just markdown\nNo frontmatter here."
	fm, body, err := ParseFrontmatter(content)
	require.NoError(t, err)
	assert.Equal(t, TaskFrontmatter{}, fm)
	assert.Equal(t, content, body)
}

func TestParseFrontmatter_Malformed(t *testing.T) {
	content := "---\nsome: yaml\nno closing delimiter"
	fm, body, err := ParseFrontmatter(content)
	require.NoError(t, err)
	assert.Equal(t, TaskFrontmatter{}, fm)
	assert.Equal(t, content, body)
}

func TestRoundtrip(t *testing.T) {
	fm := TaskFrontmatter{
		Timestamp:     time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		SessionID:     "20260115-103000",
		Outcome:       "failure",
		Agents:        []AgentOutcome{{Number: 1, Name: "Systems Engineer", Status: "success"}, {Number: 4, Name: "Software Engineer", Status: "failed"}},
		DurationSec:   120.7,
		FilesModified: []string{"main.go", "util.go"},
		TestResult:    "fail",
	}
	body := "# My Task\n\nDo something important."

	injected, err := InjectFrontmatter(fm, body)
	require.NoError(t, err)

	parsed, parsedBody, err := ParseFrontmatter(injected)
	require.NoError(t, err)
	assert.Equal(t, body, parsedBody)
	assert.Equal(t, fm.SessionID, parsed.SessionID)
	assert.Equal(t, fm.Outcome, parsed.Outcome)
	assert.Equal(t, fm.Agents, parsed.Agents)
	assert.InDelta(t, fm.DurationSec, parsed.DurationSec, 0.01)
	assert.Equal(t, fm.FilesModified, parsed.FilesModified)
	assert.Equal(t, fm.TestResult, parsed.TestResult)
	assert.True(t, fm.Timestamp.Equal(parsed.Timestamp))
}

func TestInjectFrontmatter_AgentOutcome(t *testing.T) {
	fm := TaskFrontmatter{
		Timestamp: time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
		SessionID: "20260318-120000",
		Outcome:   "success",
		Agents: []AgentOutcome{
			{Number: 1, Name: "Systems Engineer", Status: "success"},
			{Number: 2, Name: "Prompt Engineer", Status: "failed"},
			{Number: 3, Name: "Software Architect", Status: "skipped"},
			{Number: 4, Name: "Software Engineer", Status: "success"},
			{Number: 5, Name: "QA Team Leader", Status: "success"},
		},
		DurationSec: 30.0,
		TestResult:  "pass",
	}
	result, err := InjectFrontmatter(fm, "# Task")
	require.NoError(t, err)
	assert.Contains(t, result, "number: 1")
	assert.Contains(t, result, "name: Systems Engineer")
	assert.Contains(t, result, "status: success")
	assert.Contains(t, result, "status: failed")
	assert.Contains(t, result, "status: skipped")

	parsed, body, err := ParseFrontmatter(result)
	require.NoError(t, err)
	assert.Equal(t, "# Task", body)
	require.Len(t, parsed.Agents, 5)
	assert.Equal(t, 1, parsed.Agents[0].Number)
	assert.Equal(t, "Systems Engineer", parsed.Agents[0].Name)
	assert.Equal(t, "success", parsed.Agents[0].Status)
	assert.Equal(t, 2, parsed.Agents[1].Number)
	assert.Equal(t, "failed", parsed.Agents[1].Status)
	assert.Equal(t, 3, parsed.Agents[2].Number)
	assert.Equal(t, "skipped", parsed.Agents[2].Status)
}

func TestInjectFrontmatter_EmptyAgents(t *testing.T) {
	fm := TaskFrontmatter{
		Timestamp:   time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
		SessionID:   "20260318-120000",
		Outcome:     "success",
		Agents:      nil,
		DurationSec: 10.0,
	}
	result, err := InjectFrontmatter(fm, "# Task")
	require.NoError(t, err)

	parsed, body, err := ParseFrontmatter(result)
	require.NoError(t, err)
	assert.Equal(t, "# Task", body)
	assert.Empty(t, parsed.Agents)
}
