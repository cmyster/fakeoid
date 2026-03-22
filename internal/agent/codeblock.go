package agent

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// CodeBlock represents an extracted code block with file path annotation.
type CodeBlock struct {
	FilePath string
	Language string
	Content  string
}

// FileResult describes the outcome of writing a single file.
type FileResult struct {
	Path   string
	Action string // "created" or "modified"
}

// fenceOpenRegex matches an opening fence like ```go:path/to/file.go
// Group 1 = backticks, Group 2 = language, Group 3 = file path.
var fenceOpenRegex = regexp.MustCompile("^(`{3,})(\\w+):(.+)$")

// fenceCloseRegex matches a closing fence (only backticks on the line).
var fenceCloseRegex = regexp.MustCompile("^`{3,}$")

// unannotatedFenceRegex matches an opening fence without a file path annotation.
var unannotatedFenceRegex = regexp.MustCompile("^`{3,}")

// unannotatedFenceOpenRegex captures the language from an unannotated fence like ```go
var unannotatedFenceOpenRegex = regexp.MustCompile("^(`{3,})(\\w+)\\s*$")

// fileCommentRegex matches common file path comments at the start of a code block.
// Examples: "// file: path/to/file.go", "// path/to/file.go", "# file: script.py"
var fileCommentRegex = regexp.MustCompile(`^(?://|#)\s*(?:file:\s*)?(\S+\.\w+)\s*$`)

// ParseCodeBlocks extracts annotated code blocks from LLM response text.
// Blocks with the format ```language:filepath are preferred. As a fallback,
// unannotated blocks whose first line is a file path comment (e.g.,
// "// file: path/to/file.go") are also extracted.
func ParseCodeBlocks(response string) []CodeBlock {
	var blocks []CodeBlock
	var fallbackBlocks []CodeBlock
	scanner := bufio.NewScanner(strings.NewReader(response))

	var inBlock bool
	var current CodeBlock
	var openTicks int
	var content strings.Builder
	var inUnannotated bool
	var unannotatedTicks int
	var unannotatedLang string
	var unannotatedContent strings.Builder
	var unannotatedFirstLine bool

	for scanner.Scan() {
		line := scanner.Text()

		if inUnannotated {
			trimmed := strings.TrimSpace(line)
			if fenceCloseRegex.MatchString(trimmed) {
				tickCount := countLeadingBackticks(trimmed)
				if tickCount >= unannotatedTicks {
					inUnannotated = false
				}
			} else {
				// Check first content line for a file path comment
				if unannotatedFirstLine {
					unannotatedFirstLine = false
					if m := fileCommentRegex.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
						// Found file path in comment -- track as fallback block
						unannotatedContent.Reset()
						// Store the path, continue collecting content
						current = CodeBlock{
							Language: unannotatedLang,
							FilePath: m[1],
						}
						// Don't include the file comment line in content
						continue
					}
					current = CodeBlock{} // no file path found
				}
				if current.FilePath != "" {
					unannotatedContent.WriteString(line)
					unannotatedContent.WriteByte('\n')
				}
			}
			// Closing fence while tracking a fallback block
			if !inUnannotated && current.FilePath != "" {
				current.Content = unannotatedContent.String()
				fallbackBlocks = append(fallbackBlocks, current)
				current = CodeBlock{}
			}
			continue
		}

		if inBlock {
			// Check for closing fence
			trimmed := strings.TrimSpace(line)
			if fenceCloseRegex.MatchString(trimmed) {
				tickCount := countLeadingBackticks(trimmed)
				if tickCount >= openTicks {
					current.Content = content.String()
					blocks = append(blocks, current)
					inBlock = false
					continue
				}
			}
			content.WriteString(line)
			content.WriteByte('\n')
			continue
		}

		// Not inside any block -- check for opening fence
		trimmed := strings.TrimSpace(line)
		if m := fenceOpenRegex.FindStringSubmatch(trimmed); m != nil {
			openTicks = len(m[1])
			current = CodeBlock{
				Language: m[2],
				FilePath: strings.TrimSpace(m[3]),
			}
			content.Reset()
			inBlock = true
			continue
		}

		// Check for unannotated fence (fallback parsing)
		if m := unannotatedFenceOpenRegex.FindStringSubmatch(trimmed); m != nil {
			unannotatedTicks = len(m[1])
			unannotatedLang = m[2]
			unannotatedContent.Reset()
			unannotatedFirstLine = true
			inUnannotated = true
			current = CodeBlock{}
			continue
		}

		// Bare fence (no language) -- skip
		if unannotatedFenceRegex.MatchString(trimmed) && !fenceCloseRegex.MatchString(trimmed) {
			ticks := countLeadingBackticks(trimmed)
			if ticks >= 3 {
				inUnannotated = true
				unannotatedTicks = ticks
				unannotatedLang = ""
				unannotatedFirstLine = false
				current = CodeBlock{}
			}
		}
	}

	// Prefer annotated blocks; use fallbacks only if no annotated blocks found
	if len(blocks) > 0 {
		return blocks
	}
	return fallbackBlocks
}

// countLeadingBackticks returns the number of leading backtick characters.
func countLeadingBackticks(s string) int {
	count := 0
	for _, c := range s {
		if c == '`' {
			count++
		} else {
			break
		}
	}
	return count
}

// WriteCodeBlocks writes extracted code blocks to sandbox-relative paths.
// It creates directories as needed and returns a result for each file.
// Blocked writes are collected and returned alongside successful results
// (error-and-continue semantics).
func WriteCodeBlocks(sb *sandbox.Sandbox, blocks []CodeBlock) ([]FileResult, []sandbox.BlockedFile) {
	var results []FileResult
	var blocked []sandbox.BlockedFile

	for _, block := range blocks {
		// Determine created vs modified
		action := "created"
		if _, err := sb.Stat(block.FilePath); err == nil {
			action = "modified"
		}

		// Create parent directories
		dir := filepath.Dir(block.FilePath)
		if dir != "." {
			if err := sb.MkdirAll(dir, 0o755); err != nil {
				blocked = append(blocked, sandbox.BlockedFile{
					Path:   block.FilePath,
					Reason: fmt.Sprintf("mkdir: %s", err),
				})
				continue
			}
		}

		if err := sb.WriteFile(block.FilePath, []byte(block.Content), 0o644); err != nil {
			blocked = append(blocked, sandbox.BlockedFile{
				Path:   block.FilePath,
				Reason: fmt.Sprintf("write: %s", err),
			})
			continue
		}

		results = append(results, FileResult{
			Path:   block.FilePath,
			Action: action,
		})
	}

	return results, blocked
}
