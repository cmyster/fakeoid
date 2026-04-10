package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// Agent5 implements the Agent interface for the QE Engineer role.
// It reads a handoff file from Agent 4, verifies the code builds, runs smoke
// tests against task requirements, and writes unit tests.
type Agent5 struct {
	cwd              string
	taskDir          string
	handoffFile      string
	fileTree         string
	turnCount        int
	handoffContent   string
	sourceFiles      string
	readmeContent    string
	taskRequirements string // enriched prompt or task description for smoke testing
	sb               *sandbox.Sandbox
}

// NewAgent5 creates a new Agent 5 (QE Engineer) with the given working directory,
// task directory, handoff file path, sandbox, README content, and task requirements.
// It scans the file tree, reads the handoff content, extracts referenced source
// files, and reads them from disk. taskRequirements is the enriched prompt or raw
// task description used for smoke testing (may be empty).
func NewAgent5(cwd, taskDir, handoffFile string, sb *sandbox.Sandbox, readmeContent, taskRequirements string) *Agent5 {
	tree, _ := ScanFileTree(cwd, 3, 0, nil)

	handoffContent := ""
	if handoffFile != "" {
		data, err := os.ReadFile(handoffFile)
		if err == nil {
			handoffContent = string(data)
		}
	}

	// Extract file paths from handoff to read source files.
	// Cap total source content to ~16KB (~4000 tokens) to avoid exceeding
	// the LLM context window when injected into the system prompt.
	filePaths := extractFilePaths(handoffContent)
	sourceFiles := readSourceFilesWithSandbox(cwd, filePaths, sb, 16384)

	return &Agent5{
		cwd:              cwd,
		taskDir:          taskDir,
		handoffFile:      handoffFile,
		fileTree:         tree,
		handoffContent:   handoffContent,
		sourceFiles:      sourceFiles,
		readmeContent:    readmeContent,
		taskRequirements: taskRequirements,
		sb:               sb,
	}
}

// Number returns the agent's pipeline position.
func (a *Agent5) Number() int { return 5 }

// Name returns the human-readable agent name.
func (a *Agent5) Name() string { return "QE Engineer" }

// SystemPrompt returns the system prompt for Agent 5, including CWD, file tree,
// handoff content, source file contents, README build instructions, and task
// requirements for smoke testing.
func (a *Agent5) SystemPrompt() string {
	return Agent5SystemPrompt(a.cwd, a.fileTree, a.handoffContent, a.sourceFiles, a.readmeContent, a.taskRequirements)
}

// HandleResponse processes an LLM response. It increments the turn counter and
// checks for code blocks. Returns ActionComplete if code blocks are found or
// max turns reached; otherwise returns ActionContinue for multi-turn conversation.
func (a *Agent5) HandleResponse(response string) Action {
	a.turnCount++
	blocks := ParseCodeBlocks(response)
	if len(blocks) > 0 {
		return Action{Type: ActionComplete}
	}
	return Action{Type: ActionContinue}
}

// ExtractPackages parses handoff markdown content to extract unique Go package
// directory paths. It looks for lines starting with "- " that contain .go file
// paths and returns deduplicated package directories with "./" prefix.
func ExtractPackages(handoffContent string) []string {
	seen := map[string]bool{}
	var pkgs []string

	for _, line := range strings.Split(handoffContent, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		// Extract the file path (strip "- " prefix and any trailing annotation like " (created)")
		rest := strings.TrimPrefix(line, "- ")
		// Split on space to get the path before any parenthetical
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			continue
		}
		fp := parts[0]
		if !strings.HasSuffix(fp, ".go") {
			continue
		}
		dir := "./" + filepath.Dir(fp)
		if !seen[dir] {
			seen[dir] = true
			pkgs = append(pkgs, dir)
		}
	}
	return pkgs
}

// extractFilePaths parses handoff markdown to extract individual source file paths.
// Accepts any file with an extension (not just .go).
func extractFilePaths(handoffContent string) []string {
	var paths []string
	for _, line := range strings.Split(handoffContent, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		rest := strings.TrimPrefix(line, "- ")
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			continue
		}
		fp := parts[0]
		if strings.Contains(filepath.Base(fp), ".") {
			paths = append(paths, fp)
		}
	}
	return paths
}

// languageFromExt returns a markdown language tag for the given file extension.
func languageFromExt(path string) string {
	switch filepath.Ext(path) {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".md":
		return "markdown"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".sh", ".bash":
		return "bash"
	default:
		return ""
	}
}

// readSourceFiles reads the given file paths from disk relative to cwd and
// formats them as markdown code blocks for LLM context injection.
// maxBytes caps the total output size; 0 means unlimited.
func readSourceFiles(cwd string, filePaths []string, maxBytes int) string {
	var buf strings.Builder
	for _, fp := range filePaths {
		if maxBytes > 0 && buf.Len() >= maxBytes {
			fmt.Fprintf(&buf, "(remaining files omitted — context budget reached)\n")
			break
		}
		data, err := os.ReadFile(filepath.Join(cwd, fp))
		if err != nil {
			continue // skip files that can't be read
		}
		lang := languageFromExt(fp)
		entry := fmt.Sprintf("### %s\n```%s\n%s\n```\n\n", fp, lang, string(data))
		if maxBytes > 0 && buf.Len()+len(entry) > maxBytes {
			fmt.Fprintf(&buf, "(remaining files omitted — context budget reached)\n")
			break
		}
		buf.WriteString(entry)
	}
	return buf.String()
}

// readSourceFilesWithSandbox reads source files, validating each path against
// the sandbox before reading. Blocked files are silently skipped.
// maxBytes caps the total output size to avoid exceeding the LLM context window;
// 0 means unlimited.
func readSourceFilesWithSandbox(cwd string, filePaths []string, sb *sandbox.Sandbox, maxBytes int) string {
	if sb == nil {
		return readSourceFiles(cwd, filePaths, maxBytes)
	}
	var buf strings.Builder
	for _, fp := range filePaths {
		if maxBytes > 0 && buf.Len() >= maxBytes {
			fmt.Fprintf(&buf, "(remaining files omitted — context budget reached)\n")
			break
		}
		absPath := filepath.Join(cwd, fp)
		if err := sb.ValidateRead(absPath); err != nil {
			continue // blocked by sandbox
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			continue // skip files that can't be read
		}
		lang := languageFromExt(fp)
		entry := fmt.Sprintf("### %s\n```%s\n%s\n```\n\n", fp, lang, string(data))
		if maxBytes > 0 && buf.Len()+len(entry) > maxBytes {
			fmt.Fprintf(&buf, "(remaining files omitted — context budget reached)\n")
			break
		}
		buf.WriteString(entry)
	}
	return buf.String()
}

// RunGoBuild executes `go build ./...` in the given working directory to verify
// compilation. Returns the captured output, whether the build passed, and any
// execution error. Build failure (non-zero exit) is indicated by passed=false.
func RunGoBuild(ctx context.Context, cwd string, liveOut io.Writer) (string, bool, error) {
	args := []string{"build", "./..."}

	if err := sandbox.ValidateCommand("go", args); err != nil {
		return "", false, err
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = cwd

	var captured bytes.Buffer
	multi := io.MultiWriter(liveOut, &captured)
	cmd.Stdout = multi
	cmd.Stderr = multi

	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return captured.String(), false, nil
		}
		return captured.String(), false, err
	}

	return captured.String(), true, nil
}

// RunGoTest executes `go test -v -count=1` for the specified packages in the
// given working directory. It streams output to liveOut while also capturing it.
// Returns the captured output, whether tests passed, and any execution error.
// Test failure (non-zero exit) is indicated by passed=false, not by error.
func RunGoTest(ctx context.Context, cwd string, packages []string, liveOut io.Writer) (string, bool, error) {
	args := append([]string{"test", "-v", "-count=1"}, packages...)

	// Validate command before execution
	if err := sandbox.ValidateCommand("go", args); err != nil {
		return "", false, err
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = cwd

	var captured bytes.Buffer
	multi := io.MultiWriter(liveOut, &captured)
	cmd.Stdout = multi
	cmd.Stderr = multi

	err := cmd.Run()
	passed := err == nil

	// If the error is an ExitError (test failure), don't propagate as error
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return captured.String(), false, nil
		}
		// Real execution error (e.g., go binary not found)
		return captured.String(), false, err
	}

	return captured.String(), passed, nil
}

// ReadBuildInstructions reads README.md from the project root and returns its
// content. Returns empty string if README.md does not exist or cannot be read.
func ReadBuildInstructions(cwd string, sb *sandbox.Sandbox) string {
	readmePath := filepath.Join(cwd, "README.md")
	if sb != nil {
		if err := sb.ValidateRead(readmePath); err != nil {
			return ""
		}
	}
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return ""
	}
	return string(data)
}

// IsGoProject checks whether the given directory contains a go.mod file.
func IsGoProject(cwd string) bool {
	_, err := os.Stat(filepath.Join(cwd, "go.mod"))
	return err == nil
}

// parseBuildCommand extracts the first shell command from a README's Build section.
// It looks for ## Build or ### Build headings, then extracts the first fenced code
// block or inline code after that heading.
func parseBuildCommand(readmeContent string) string {
	lines := strings.Split(readmeContent, "\n")
	inBuildSection := false
	inFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Look for build heading
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "## build") || strings.HasPrefix(lower, "### build") {
			inBuildSection = true
			continue
		}

		// Stop at next heading
		if inBuildSection && strings.HasPrefix(trimmed, "#") && !inFence {
			break
		}

		if !inBuildSection {
			continue
		}

		// Track fenced code blocks
		if strings.HasPrefix(trimmed, "```") {
			if inFence {
				// End of fence — we already captured what we needed
				break
			}
			inFence = true
			continue
		}

		if inFence && trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// RunBuildAndVerify attempts to build the project using commands from README.md.
// If no README build section is found, falls back to language-marker detection
// (Cargo.toml -> cargo build, Makefile -> make, etc.).
// Returns the captured output, whether the build passed, and any execution error.
func RunBuildAndVerify(ctx context.Context, cwd string, readmeContent string, liveOut io.Writer) (string, bool, error) {
	// Try to parse build command from README
	buildCmd := parseBuildCommand(readmeContent)

	// Fallback: detect from project files
	if buildCmd == "" {
		switch {
		case fileExists(filepath.Join(cwd, "Cargo.toml")):
			buildCmd = "cargo build"
		case fileExists(filepath.Join(cwd, "Makefile")):
			buildCmd = "make"
		case fileExists(filepath.Join(cwd, "package.json")):
			buildCmd = "npm run build"
		default:
			return "no build instructions found in README.md and no recognized project files", false, nil
		}
	}

	// Split command into name and args
	parts := strings.Fields(buildCmd)
	if len(parts) == 0 {
		return "empty build command", false, nil
	}
	name := parts[0]
	args := parts[1:]

	// Validate command before execution
	if err := sandbox.ValidateCommand(name, args); err != nil {
		return "", false, err
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd

	var captured bytes.Buffer
	multi := io.MultiWriter(liveOut, &captured)
	cmd.Stdout = multi
	cmd.Stderr = multi

	err := cmd.Run()
	passed := err == nil

	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return captured.String(), false, nil
		}
		return captured.String(), false, err
	}

	return captured.String(), passed, nil
}

// RunTestScript runs test.sh if Agent 5 created one, otherwise falls back to
// RunBuildAndVerify. test.sh is the preferred verification method because it
// contains build + run + output checks written by Agent 5 for this specific project.
func RunTestScript(ctx context.Context, cwd string, readmeContent string, liveOut io.Writer) (string, bool, error) {
	testScript := filepath.Join(cwd, "test.sh")
	if fileExists(testScript) {
		// Make executable
		os.Chmod(testScript, 0o755)

		if err := sandbox.ValidateCommand("bash", []string{testScript}); err != nil {
			return "", false, err
		}

		cmd := exec.CommandContext(ctx, "bash", testScript)
		cmd.Dir = cwd

		var captured bytes.Buffer
		multi := io.MultiWriter(liveOut, &captured)
		cmd.Stdout = multi
		cmd.Stderr = multi

		err := cmd.Run()
		passed := err == nil

		if err != nil {
			if _, ok := err.(*exec.ExitError); ok {
				return captured.String(), false, nil
			}
			return captured.String(), false, err
		}
		return captured.String(), passed, nil
	}

	// No test.sh — fall back to README-based build verification
	return RunBuildAndVerify(ctx, cwd, readmeContent, liveOut)
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
