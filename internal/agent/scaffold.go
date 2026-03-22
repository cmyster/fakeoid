package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// filePathRegex matches file-like paths: must contain a slash or dot-extension,
// and end with a known source extension. Captures from backtick-wrapped paths.
var filePathRegex = regexp.MustCompile("`([a-zA-Z0-9_./-]+\\.[a-zA-Z0-9]+)`")

// bareFilePathRegex matches bare paths not in backticks (lines starting with - or *)
var bareFilePathRegex = regexp.MustCompile(`(?:^|\s)((?:[a-zA-Z0-9_-]+/)+[a-zA-Z0-9_-]+\.[a-zA-Z0-9]+)`)

// knownExtensions are file extensions we consider valid for scaffolding.
var knownExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true,
	".jsx": true, ".rs": true, ".java": true, ".yaml": true, ".yml": true,
	".json": true, ".toml": true, ".mod": true, ".sum": true,
	".css": true, ".html": true, ".sql": true, ".sh": true,
	".md": true,
}

// ExtractFilePaths parses markdown text (typically a change plan) and extracts
// file paths. It looks for backtick-wrapped paths and bare paths that look like
// file references (contain a slash and have a known extension).
// Returns deduplicated paths in order of first appearance.
func ExtractFilePaths(markdown string) []string {
	seen := map[string]bool{}
	var paths []string

	for _, line := range strings.Split(markdown, "\n") {
		// Try backtick-wrapped paths first
		for _, m := range filePathRegex.FindAllStringSubmatch(line, -1) {
			p := m[1]
			if isFilePath(p) && !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
		// Try bare paths
		for _, m := range bareFilePathRegex.FindAllStringSubmatch(line, -1) {
			p := strings.TrimSpace(m[1])
			if isFilePath(p) && !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}

	return paths
}

// isFilePath checks whether a string looks like a real file path:
// must have a known extension.
func isFilePath(s string) bool {
	ext := filepath.Ext(s)
	if ext == "" {
		return false
	}
	return knownExtensions[ext]
}

// ScaffoldFiles creates directories and placeholder files for the given paths.
// For new files: creates parent directories and an empty file with a package
// declaration (for .go files) or empty content. For existing files: prepends
// a TODO comment. Errors are collected and returned, never fatal.
func ScaffoldFiles(sb *sandbox.Sandbox, paths []string) []error {
	var errs []error

	for _, p := range paths {
		dir := filepath.Dir(p)
		if dir != "." {
			if err := sb.MkdirAll(dir, 0o755); err != nil {
				errs = append(errs, fmt.Errorf("scaffold mkdir %s: %w", dir, err))
				continue
			}
		}

		// Check if file exists
		_, statErr := sb.Stat(p)
		if statErr == nil {
			// Existing file: prepend TODO comment
			absPath := filepath.Join(sb.CWD(), p)
			existing, readErr := os.ReadFile(absPath)
			if readErr != nil {
				errs = append(errs, fmt.Errorf("scaffold read %s: %w", p, readErr))
				continue
			}
			comment := "// TODO: Agent 3 flagged for modification\n"
			if err := sb.WriteFile(p, []byte(comment+string(existing)), 0o644); err != nil {
				errs = append(errs, fmt.Errorf("scaffold write %s: %w", p, err))
			}
			continue
		}

		// New file: create with package declaration for .go files
		var content string
		if filepath.Ext(p) == ".go" {
			pkgName := filepath.Base(filepath.Dir(p))
			if pkgName == "." || pkgName == "" {
				pkgName = "main"
			}
			content = fmt.Sprintf("package %s\n", pkgName)
		}

		if err := sb.WriteFile(p, []byte(content), 0o644); err != nil {
			errs = append(errs, fmt.Errorf("scaffold write %s: %w", p, err))
		}
	}

	return errs
}
