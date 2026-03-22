package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadHistory_Empty(t *testing.T) {
	idx, err := LoadHistory("/nonexistent/path/history.json")
	require.NoError(t, err)
	assert.Empty(t, idx.Records)
}

func TestLoadHistory_Corrupt(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "history.json")
	require.NoError(t, os.WriteFile(path, []byte("{corrupt json!!!"), 0o644))

	idx, err := LoadHistory(path)
	require.NoError(t, err)
	assert.Empty(t, idx.Records)
}

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	sb, err := sandbox.New(tmp)
	require.NoError(t, err)
	defer sb.Close()

	index := HistoryIndex{
		Records: []HistoryRecord{
			{
				SessionID: "20260316-120000",
				Timestamp: time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC),
				TaskName:  "test task",
				Outcome:   "success",
				TaskFile:  "tasks/001-test-task.md",
			},
		},
	}

	err = SaveHistory(sb, "history.json", index)
	require.NoError(t, err)

	loaded, err := LoadHistory(filepath.Join(tmp, "history.json"))
	require.NoError(t, err)
	require.Len(t, loaded.Records, 1)
	assert.Equal(t, "test task", loaded.Records[0].TaskName)
	assert.Equal(t, "success", loaded.Records[0].Outcome)
}

func TestAppendRecord(t *testing.T) {
	tmp := t.TempDir()
	sb, err := sandbox.New(tmp)
	require.NoError(t, err)
	defer sb.Close()

	histDir := filepath.Join(tmp, ".fakeoid")
	require.NoError(t, os.MkdirAll(histDir, 0o755))

	// Create initial history
	initial := HistoryIndex{
		Records: []HistoryRecord{
			{SessionID: "s1", TaskName: "first", Outcome: "success", TaskFile: "tasks/001.md"},
		},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(histDir, "history.json"), data, 0o644))

	// Append a new record
	rec := HistoryRecord{
		SessionID: "s2",
		Timestamp: time.Now(),
		TaskName:  "second",
		Outcome:   "failure",
		TaskFile:  "tasks/002.md",
	}
	err = AppendRecord(sb, histDir, rec)
	require.NoError(t, err)

	loaded, err := LoadHistory(filepath.Join(histDir, "history.json"))
	require.NoError(t, err)
	require.Len(t, loaded.Records, 2)
	assert.Equal(t, "first", loaded.Records[0].TaskName)
	assert.Equal(t, "second", loaded.Records[1].TaskName)
}

func TestAppendRecord_NewFile(t *testing.T) {
	tmp := t.TempDir()
	sb, err := sandbox.New(tmp)
	require.NoError(t, err)
	defer sb.Close()

	histDir := filepath.Join(tmp, ".fakeoid")
	require.NoError(t, os.MkdirAll(histDir, 0o755))

	rec := HistoryRecord{
		SessionID: "s1",
		TaskName:  "first",
		Outcome:   "success",
		TaskFile:  "tasks/001.md",
	}
	err = AppendRecord(sb, histDir, rec)
	require.NoError(t, err)

	loaded, err := LoadHistory(filepath.Join(histDir, "history.json"))
	require.NoError(t, err)
	require.Len(t, loaded.Records, 1)
	assert.Equal(t, "first", loaded.Records[0].TaskName)
}

func TestRebuildFromTaskFiles(t *testing.T) {
	tmp := t.TempDir()
	taskDir := filepath.Join(tmp, "tasks")
	require.NoError(t, os.MkdirAll(taskDir, 0o755))

	// Create task files with frontmatter
	fm1 := TaskFrontmatter{
		Timestamp:   time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		SessionID:   "20260315-100000",
		Outcome:     "success",
		Agents:      []AgentOutcome{{Number: 1, Name: "Systems Engineer", Status: "success"}, {Number: 4, Name: "Software Engineer", Status: "success"}, {Number: 5, Name: "QE Engineer", Status: "success"}},
		DurationSec: 30,
	}
	fm2 := TaskFrontmatter{
		Timestamp:   time.Date(2026, 3, 16, 14, 0, 0, 0, time.UTC),
		SessionID:   "20260316-140000",
		Outcome:     "failure",
		Agents:      []AgentOutcome{{Number: 1, Name: "Systems Engineer", Status: "success"}, {Number: 4, Name: "Software Engineer", Status: "failed"}},
		DurationSec: 60,
	}

	content1, _ := InjectFrontmatter(fm1, "# Task One")
	content2, _ := InjectFrontmatter(fm2, "# Task Two")
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "001-task-one.md"), []byte(content1), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "002-task-two.md"), []byte(content2), 0o644))

	idx, err := RebuildFromTaskFiles(taskDir)
	require.NoError(t, err)
	require.Len(t, idx.Records, 2)

	// Should be sorted by timestamp (older first)
	assert.Equal(t, "20260315-100000", idx.Records[0].SessionID)
	assert.Equal(t, "20260316-140000", idx.Records[1].SessionID)
	assert.Equal(t, "success", idx.Records[0].Outcome)
	assert.Equal(t, "failure", idx.Records[1].Outcome)
}

func TestEnsureGitignore_NoFile(t *testing.T) {
	tmp := t.TempDir()
	sb, err := sandbox.New(tmp)
	require.NoError(t, err)
	defer sb.Close()

	// No .gitignore file exists
	err = EnsureGitignore(sb)
	assert.NoError(t, err)

	// Should not have created .gitignore
	_, err = os.Stat(filepath.Join(tmp, ".gitignore"))
	assert.True(t, os.IsNotExist(err))
}

func TestEnsureGitignore_AlreadyPresent(t *testing.T) {
	tmp := t.TempDir()
	content := "node_modules/\n.fakeoid/\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte(content), 0o644))

	sb, err := sandbox.New(tmp)
	require.NoError(t, err)
	defer sb.Close()

	err = EnsureGitignore(sb)
	assert.NoError(t, err)

	// Content should be unchanged
	got, _ := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	assert.Equal(t, content, string(got))
}

func TestEnsureGitignore_Appends(t *testing.T) {
	tmp := t.TempDir()
	content := "node_modules/\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte(content), 0o644))

	sb, err := sandbox.New(tmp)
	require.NoError(t, err)
	defer sb.Close()

	err = EnsureGitignore(sb)
	assert.NoError(t, err)

	got, _ := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	assert.Contains(t, string(got), ".fakeoid/")
	assert.Contains(t, string(got), "node_modules/")
}

func TestClearHistory(t *testing.T) {
	tmp := t.TempDir()
	fakeoidDir := filepath.Join(tmp, ".fakeoid")
	taskDir := filepath.Join(fakeoidDir, "tasks")
	require.NoError(t, os.MkdirAll(taskDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "001.md"), []byte("task"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(fakeoidDir, "history.json"), []byte("{}"), 0o644))

	err := ClearHistory(fakeoidDir)
	assert.NoError(t, err)

	_, err = os.Stat(taskDir)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(fakeoidDir, "history.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestSessionID(t *testing.T) {
	id := GenerateSessionID()
	// Should match YYYYMMDD-HHMMSS format
	matched, err := regexp.MatchString(`^\d{8}-\d{6}$`, id)
	require.NoError(t, err)
	assert.True(t, matched, "session ID %q should match YYYYMMDD-HHMMSS", id)
}
