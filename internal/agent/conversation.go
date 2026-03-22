package agent

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/cmyster/fakeoid/internal/server"
	"gopkg.in/yaml.v3"
)

// ConversationFrontmatter holds metadata for conversation audit trail files.
type ConversationFrontmatter struct {
	AgentNumber int       `yaml:"agent_number"`
	AgentName   string    `yaml:"agent_name"`
	Timestamp   time.Time `yaml:"timestamp"`
	DurationSec float64   `yaml:"duration_sec"`
}

// marshalConversationFrontmatter serializes the frontmatter to a YAML string.
func marshalConversationFrontmatter(fm ConversationFrontmatter) string {
	data, err := yaml.Marshal(fm)
	if err != nil {
		return ""
	}
	return string(data)
}

// FormatConversation renders a slice of server.Message as a markdown string.
//
// The first message (role="system") is rendered under "## System Prompt".
// Remaining messages are grouped into user+assistant pairs:
//   - If exactly one pair: rendered under "## Conversation" with "### User" and "### Assistant"
//   - If multiple pairs: rendered as "## Turn 1", "## Turn 2", etc. with "### User"/"### Assistant"
//   - If no remaining messages after system: only the system prompt section is rendered
//
// Empty or nil input returns an empty string.
func FormatConversation(messages []server.Message) string {
	if len(messages) == 0 {
		return ""
	}

	var sb strings.Builder

	// First message should be the system prompt
	if messages[0].Role == "system" {
		sb.WriteString("## System Prompt\n\n")
		sb.WriteString(messages[0].Content)
		sb.WriteString("\n\n")
		messages = messages[1:]
	}

	if len(messages) == 0 {
		return sb.String()
	}

	// Group remaining messages into user+assistant pairs
	type pair struct {
		user      string
		assistant string
	}
	var pairs []pair
	for i := 0; i < len(messages); i += 2 {
		p := pair{}
		if i < len(messages) {
			p.user = messages[i].Content
		}
		if i+1 < len(messages) {
			p.assistant = messages[i+1].Content
		}
		pairs = append(pairs, p)
	}

	if len(pairs) == 1 {
		// Single-turn: flat Conversation section
		sb.WriteString("## Conversation\n\n")
		sb.WriteString("### User\n\n")
		sb.WriteString(pairs[0].user)
		sb.WriteString("\n\n")
		sb.WriteString("### Assistant\n\n")
		sb.WriteString(pairs[0].assistant)
		sb.WriteString("\n\n")
	} else {
		// Multi-turn: numbered Turn sections
		for i, p := range pairs {
			sb.WriteString(fmt.Sprintf("## Turn %d\n\n", i+1))
			sb.WriteString("### User\n\n")
			sb.WriteString(p.user)
			sb.WriteString("\n\n")
			sb.WriteString("### Assistant\n\n")
			sb.WriteString(p.assistant)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// WriteConversationFile creates a conversation audit trail file through the sandbox.
//
// File naming:
//   - iteration=0: NNN-slug-agentN-conversation.md
//   - iteration>0: NNN-slug-agentN-iterM-conversation.md
//
// The file contains YAML frontmatter (agent_number, agent_name, timestamp, duration_sec)
// followed by the formatted conversation markdown.
//
// Returns the absolute path to the written file, or an error.
func WriteConversationFile(sb *sandbox.Sandbox, taskDir, taskFileName string, agentNum int, agentName string, iteration int, messages []server.Message, duration time.Duration) (string, error) {
	if sb == nil {
		return "", fmt.Errorf("sandbox is nil")
	}

	// Derive filename
	base := strings.TrimSuffix(taskFileName, ".md")
	var fileName string
	if iteration > 0 {
		fileName = fmt.Sprintf("%s-agent%d-iter%d-conversation.md", base, agentNum, iteration)
	} else {
		fileName = fmt.Sprintf("%s-agent%d-conversation.md", base, agentNum)
	}
	filePath := filepath.Join(taskDir, fileName)

	// Build frontmatter
	fm := ConversationFrontmatter{
		AgentNumber: agentNum,
		AgentName:   agentName,
		Timestamp:   time.Now().UTC(),
		DurationSec: duration.Seconds(),
	}
	yamlData := marshalConversationFrontmatter(fm)

	// Build content
	body := FormatConversation(messages)
	content := "---\n" + yamlData + "---\n" + body

	// Write through sandbox
	relTaskDir, err := filepath.Rel(sb.CWD(), taskDir)
	if err != nil {
		relTaskDir = taskDir
	}
	relFilePath, err := filepath.Rel(sb.CWD(), filePath)
	if err != nil {
		relFilePath = filePath
	}

	if err := sb.MkdirAll(relTaskDir, 0o755); err != nil {
		return "", fmt.Errorf("create task directory: %w", err)
	}
	if err := sb.WriteFile(relFilePath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write conversation file: %w", err)
	}

	return filePath, nil
}
