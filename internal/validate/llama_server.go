package validate

import (
	"fmt"
	"strings"
)

// CheckLlamaServer verifies that llama-server is available in PATH.
func CheckLlamaServer(runner CommandRunner, verbose bool) (*CheckResult, error) {
	result := &CheckResult{Name: "llama-server"}

	path, err := runner.LookPath("llama-server")
	if err != nil {
		result.Passed = false
		result.Error = "llama-server not found in PATH"
		return result, nil
	}

	result.Passed = true
	if verbose {
		result.Detail = fmt.Sprintf("llama-server: %s", path)
	}
	return result, nil
}

// CheckROCmBuild verifies that the llama-server binary is linked against libamdhip64.
func CheckROCmBuild(runner CommandRunner, llamaServerPath string, verbose bool) (*CheckResult, error) {
	result := &CheckResult{Name: "rocm-build"}

	output, err := runner.Run("ldd", llamaServerPath)
	if err != nil {
		result.Passed = false
		result.Error = fmt.Sprintf("failed to run ldd on %s: %s", llamaServerPath, err)
		return result, nil
	}

	if !strings.Contains(string(output), "libamdhip64") {
		result.Passed = false
		result.Error = "llama-server is not built with ROCm support (libamdhip64 not linked)"
		return result, nil
	}

	result.Passed = true
	if verbose {
		result.Detail = "ROCm build: libamdhip64 linked"
	}
	return result, nil
}
