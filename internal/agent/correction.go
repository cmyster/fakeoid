package agent

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cmyster/fakeoid/internal/sandbox"
)

// WriteCorrectionFile creates a correction directive markdown file through the sandbox.
// The file is named by stripping .md from taskFileName and appending
// "-correction-iterN.md". Always iteration-numbered per D-06.
func WriteCorrectionFile(sb *sandbox.Sandbox, taskDir, taskFileName string, iteration int, content string) (string, error) {
	if sb == nil {
		return "", fmt.Errorf("sandbox is nil")
	}
	base := strings.TrimSuffix(taskFileName, ".md")
	correctionName := fmt.Sprintf("%s-correction-iter%d.md", base, iteration)
	correctionPath := filepath.Join(taskDir, correctionName)

	relTaskDir, err := filepath.Rel(sb.CWD(), taskDir)
	if err != nil {
		relTaskDir = taskDir
	}
	relCorrectionPath, err := filepath.Rel(sb.CWD(), correctionPath)
	if err != nil {
		relCorrectionPath = correctionPath
	}

	if err := sb.MkdirAll(relTaskDir, 0o755); err != nil {
		return "", fmt.Errorf("create task directory: %w", err)
	}
	if err := sb.WriteFile(relCorrectionPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write correction file: %w", err)
	}
	return correctionPath, nil
}
