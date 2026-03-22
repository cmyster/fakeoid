package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// WriteTaskFile creates a numbered task prompt file in the given directory.
// The directory is created if it does not exist. Files are numbered sequentially
// in NNN format (001, 002, ...). Returns the path of the created file.
// Writes are routed through the sandbox.
func WriteTaskFile(sb *sandbox.Sandbox, taskDir string, content string) (string, error) {
	// Compute sandbox-relative path for directory creation
	relTaskDir, err := filepath.Rel(sb.CWD(), taskDir)
	if err != nil {
		relTaskDir = taskDir
	}

	if err := sb.MkdirAll(relTaskDir, 0o755); err != nil {
		return "", fmt.Errorf("create task directory: %w", err)
	}

	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return "", fmt.Errorf("read task directory: %w", err)
	}

	num := len(entries) + 1
	slug := generateSlug(content, 0)
	filename := fmt.Sprintf("%03d-%s-task.md", num, slug)
	path := filepath.Join(taskDir, filename)

	relPath, err := filepath.Rel(sb.CWD(), path)
	if err != nil {
		relPath = path
	}

	if err := sb.WriteFile(relPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write task file: %w", err)
	}
	return path, nil
}

var slugRegexp = regexp.MustCompile(`[^a-z0-9]+`)

// generateSlug creates a filename-safe slug from text.
// It uses the first line, lowercases it, strips markdown heading markers,
// replaces non-alphanumeric characters with hyphens, and truncates at maxLen chars.
// maxLen of 0 defaults to 50.
func generateSlug(text string, maxLen int) string {
	if maxLen == 0 {
		maxLen = 50
	}

	line := strings.SplitN(text, "\n", 2)[0]
	line = strings.TrimSpace(line)

	// Remove markdown heading markers (# ## ### etc.)
	line = strings.TrimLeft(line, "#")
	line = strings.TrimSpace(line)

	line = strings.ToLower(line)

	slug := slugRegexp.ReplaceAllString(line, "-")
	slug = strings.Trim(slug, "-")

	if len(slug) > maxLen {
		slug = slug[:maxLen]
		// Don't end on a partial word -- find last hyphen
		if idx := strings.LastIndex(slug, "-"); idx > maxLen/2 {
			slug = slug[:idx]
		}
	}

	if slug == "" {
		slug = "task"
	}
	return slug
}
