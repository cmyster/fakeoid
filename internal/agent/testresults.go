package agent

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// WriteTestResultsFile creates a test results summary file through the sandbox.
// The file is named by stripping .md from taskFileName and replacing the -task
// suffix with -tests-iterN.md. Content is a summary: overall pass/fail,
// count of tests, and list of test file paths written.
func WriteTestResultsFile(sb *sandbox.Sandbox, taskDir, taskFileName string, iteration int, passed bool, testCount int, testFiles []string) (string, error) {
	base := strings.TrimSuffix(taskFileName, ".md")
	resultsName := fmt.Sprintf("%s-tests-iter%d.md", base, iteration)
	resultsPath := filepath.Join(taskDir, resultsName)

	// Build content
	result := "fail"
	if passed {
		result = "pass"
	}

	var content strings.Builder
	content.WriteString("# Test Results\n\n")
	fmt.Fprintf(&content, "**Result:** %s\n", result)
	fmt.Fprintf(&content, "**Tests run:** %d\n", testCount)
	content.WriteString("\n## Test Files\n\n")
	if len(testFiles) > 0 {
		for _, f := range testFiles {
			fmt.Fprintf(&content, "- %s\n", f)
		}
	} else {
		content.WriteString("- No test files written\n")
	}

	// Compute sandbox-relative paths
	relTaskDir, err := filepath.Rel(sb.CWD(), taskDir)
	if err != nil {
		relTaskDir = taskDir
	}
	relResultsPath, err := filepath.Rel(sb.CWD(), resultsPath)
	if err != nil {
		relResultsPath = resultsPath
	}

	if err := sb.MkdirAll(relTaskDir, 0o755); err != nil {
		return "", fmt.Errorf("create task directory: %w", err)
	}
	if err := sb.WriteFile(relResultsPath, []byte(content.String()), 0o644); err != nil {
		return "", fmt.Errorf("write test results file: %w", err)
	}

	return resultsPath, nil
}
