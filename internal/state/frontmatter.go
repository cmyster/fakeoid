// Package state manages persistent task history and YAML frontmatter operations.
package state

import (
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentOutcome captures the execution result of a single agent in the pipeline.
type AgentOutcome struct {
	Number int    `yaml:"number"`
	Name   string `yaml:"name"`
	Status string `yaml:"status"` // "success", "skipped", "failed"
}

// TaskFrontmatter holds metadata for task markdown files.
type TaskFrontmatter struct {
	Timestamp     time.Time      `yaml:"timestamp"`
	SessionID     string         `yaml:"session_id"`
	Outcome       string         `yaml:"outcome"`
	Agents        []AgentOutcome `yaml:"agents"`
	DurationSec   float64        `yaml:"duration_sec"`
	FilesModified []string       `yaml:"files_modified,omitempty"`
	TestResult    string         `yaml:"test_result,omitempty"`
}

// InjectFrontmatter prepends YAML frontmatter to markdown content.
// The result is "---\n{yaml}\n---\n{content}".
func InjectFrontmatter(fm TaskFrontmatter, content string) (string, error) {
	data, err := yaml.Marshal(fm)
	if err != nil {
		return "", err
	}
	return "---\n" + string(data) + "---\n" + content, nil
}

// ParseFrontmatter extracts YAML frontmatter from markdown content.
// If no valid frontmatter is found, returns zero-value struct and the original content.
func ParseFrontmatter(content string) (TaskFrontmatter, string, error) {
	if !strings.HasPrefix(content, "---\n") {
		return TaskFrontmatter{}, content, nil
	}
	end := strings.Index(content[4:], "\n---\n")
	if end == -1 {
		return TaskFrontmatter{}, content, nil
	}
	var fm TaskFrontmatter
	if err := yaml.Unmarshal([]byte(content[4:4+end]), &fm); err != nil {
		return TaskFrontmatter{}, content, nil
	}
	body := content[4+end+5:] // skip past "\n---\n"
	return fm, body, nil
}
