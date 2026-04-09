package validate

import (
	"fmt"
	"strconv"
	"strings"
)

// CheckROCmInfo verifies that rocminfo is available in PATH.
func CheckROCmInfo(runner CommandRunner, verbose bool) (*CheckResult, error) {
	result := &CheckResult{Name: "rocminfo"}

	path, err := runner.LookPath("rocminfo")
	if err != nil {
		result.Passed = false
		result.Error = "rocminfo not found in PATH"
		return result, nil
	}

	result.Passed = true
	if verbose {
		result.Detail = fmt.Sprintf("rocminfo: %s", path)
	}
	return result, nil
}

// CheckGPU runs rocminfo and parses the output for GPU agents.
// Returns the check result and a slice of detected GPUs.
func CheckGPU(runner CommandRunner, verbose bool) (*CheckResult, []GPUInfo, error) {
	result := &CheckResult{Name: "gpu"}

	output, err := runner.Run("rocminfo")
	if err != nil {
		result.Passed = false
		result.Error = fmt.Sprintf("failed to run rocminfo: %s", err)
		return result, nil, nil
	}

	gpus, err := parseROCmInfo(string(output))
	if err != nil {
		result.Passed = false
		result.Error = fmt.Sprintf("failed to parse rocminfo output: %s", err)
		return result, nil, nil
	}

	if len(gpus) == 0 {
		result.Passed = false
		result.Error = "no AMD GPU detected"
		return result, nil, nil
	}

	result.Passed = true
	if verbose {
		var details []string
		for _, gpu := range gpus {
			vramGB := float64(gpu.VRAMSizeKB) / 1024 / 1024
			details = append(details, fmt.Sprintf("GPU: %s (%s, %.0fGB VRAM)", gpu.Name, gpu.MarketingName, vramGB))
		}
		result.Detail = strings.Join(details, "; ")
	}
	return result, gpus, nil
}

// parseROCmInfo splits rocminfo output into agent blocks and extracts GPU info.
func parseROCmInfo(output string) ([]GPUInfo, error) {
	var gpus []GPUInfo

	// Split on agent block separators (lines of asterisks)
	blocks := splitAgentBlocks(output)
	for _, block := range blocks {
		if !isGPUAgent(block) {
			continue
		}
		gpu := GPUInfo{
			Name:          extractField(block, "Name:"),
			MarketingName: extractField(block, "Marketing Name:"),
			VRAMSizeKB:    extractCoarseGrainedSize(block),
			ComputeUnits:  extractComputeUnits(block),
		}
		gpus = append(gpus, gpu)
	}

	return gpus, nil
}

// splitAgentBlocks splits rocminfo output into individual agent blocks
// separated by lines of asterisks ("*******").
func splitAgentBlocks(output string) []string {
	lines := strings.Split(output, "\n")
	var blocks []string
	var current []string
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "****") {
			if inBlock && len(current) > 0 {
				blocks = append(blocks, strings.Join(current, "\n"))
				current = nil
			}
			inBlock = true
			continue
		}
		if inBlock {
			current = append(current, line)
		}
	}
	// Append last block
	if len(current) > 0 {
		blocks = append(blocks, strings.Join(current, "\n"))
	}

	return blocks
}

// isGPUAgent checks if an agent block describes a GPU device.
func isGPUAgent(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Device Type:") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "Device Type:"))
			return value == "GPU"
		}
	}
	return false
}

// extractField extracts a value from a key-value line in the agent block.
func extractField(block, key string) string {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key) {
			value := strings.TrimPrefix(trimmed, key)
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// extractComputeUnits extracts the Compute Unit count from an agent block.
// Returns 0 if the field is missing or unparseable.
func extractComputeUnits(block string) int {
	cuStr := extractField(block, "Compute Unit:")
	if cuStr == "" {
		return 0
	}
	cu, err := strconv.Atoi(strings.TrimSpace(cuStr))
	if err != nil {
		return 0
	}
	return cu
}

// extractCoarseGrainedSize extracts the VRAM size in KB from the first
// COARSE GRAINED pool in an agent block.
func extractCoarseGrainedSize(block string) uint64 {
	lines := strings.Split(block, "\n")
	foundCoarseGrained := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Look for COARSE GRAINED segment
		if strings.Contains(trimmed, "COARSE GRAINED") {
			foundCoarseGrained = true
			continue
		}

		// After finding COARSE GRAINED, look for Size:
		if foundCoarseGrained && strings.HasPrefix(trimmed, "Size:") {
			sizeStr := strings.TrimPrefix(trimmed, "Size:")
			sizeStr = strings.TrimSpace(sizeStr)
			// Format: "25149440(0x17fc000) KB" - extract the decimal number
			if parenIdx := strings.Index(sizeStr, "("); parenIdx > 0 {
				sizeStr = sizeStr[:parenIdx]
			}
			size, err := strconv.ParseUint(strings.TrimSpace(sizeStr), 10, 64)
			if err != nil {
				return 0
			}
			return size
		}

		// If we find another Segment line, we've passed the COARSE GRAINED pool
		if foundCoarseGrained && strings.HasPrefix(trimmed, "Segment:") {
			foundCoarseGrained = false
		}
	}
	return 0
}
