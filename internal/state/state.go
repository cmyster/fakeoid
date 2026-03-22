package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// HistoryRecord represents a single task entry in the history index.
type HistoryRecord struct {
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	TaskName  string    `json:"task_name"`
	Outcome   string    `json:"outcome"`
	TaskFile  string    `json:"task_file"`
}

// HistoryIndex holds all history records.
type HistoryIndex struct {
	Records []HistoryRecord `json:"records"`
}

// GenerateSessionID returns a timestamp-based session ID in "YYYYMMDD-HHMMSS" format.
func GenerateSessionID() string {
	return time.Now().Format("20060102-150405")
}

// LoadHistory reads the history index from a JSON file.
// Returns an empty index (no error) if the file is missing or corrupt.
func LoadHistory(path string) (HistoryIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return HistoryIndex{}, nil
	}
	var idx HistoryIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return HistoryIndex{}, nil
	}
	return idx, nil
}

// SaveHistory writes the history index to a JSON file via sandbox.
func SaveHistory(sb *sandbox.Sandbox, relPath string, index HistoryIndex) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return sb.WriteFile(relPath, data, 0o644)
}

// AppendRecord loads the existing history index from historyDir/history.json,
// appends the given record, and saves via sandbox.
func AppendRecord(sb *sandbox.Sandbox, historyDir string, record HistoryRecord) error {
	histPath := filepath.Join(historyDir, "history.json")
	idx, _ := LoadHistory(histPath)
	idx.Records = append(idx.Records, record)

	relPath, err := filepath.Rel(sb.CWD(), histPath)
	if err != nil {
		return err
	}
	return SaveHistory(sb, relPath, idx)
}

// RebuildFromTaskFiles reads all .md files in taskDir, parses frontmatter,
// and builds a HistoryIndex sorted by timestamp.
func RebuildFromTaskFiles(taskDir string) (HistoryIndex, error) {
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return HistoryIndex{}, err
	}

	var records []HistoryRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		// Skip artifact files -- only process task files
		name := entry.Name()
		if strings.Contains(name, "-enriched.md") || strings.Contains(name, "-handoff.md") || strings.Contains(name, "-change-plan.md") || strings.Contains(name, "-conversation.md") || strings.Contains(name, "-tests-iter") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(taskDir, entry.Name()))
		if err != nil {
			continue
		}
		fm, _, err := ParseFrontmatter(string(data))
		if err != nil || fm.SessionID == "" {
			continue
		}
		records = append(records, HistoryRecord{
			SessionID: fm.SessionID,
			Timestamp: fm.Timestamp,
			TaskName:  strings.TrimSuffix(entry.Name(), ".md"),
			Outcome:   fm.Outcome,
			TaskFile:  filepath.Join("tasks", entry.Name()),
		})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})

	return HistoryIndex{Records: records}, nil
}

// ClearHistory removes the tasks subdirectory and history.json from fakeoidDir.
// Does not error if already clean.
func ClearHistory(fakeoidDir string) error {
	if err := os.RemoveAll(filepath.Join(fakeoidDir, "tasks")); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(fakeoidDir, "history.json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// EnsureGitignore appends ".fakeoid/" to .gitignore if the file exists
// and doesn't already mention .fakeoid. If no .gitignore exists, does nothing.
func EnsureGitignore(sb *sandbox.Sandbox) error {
	gitignorePath := filepath.Join(sb.CWD(), ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.Contains(string(content), ".fakeoid") {
		return nil
	}
	updated := string(content)
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += ".fakeoid/\n"
	return sb.WriteFile(".gitignore", []byte(updated), 0o644)
}
