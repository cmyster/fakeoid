package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecRunnerImplementsCommandRunner(t *testing.T) {
	// Compile-time check: ExecRunner must implement CommandRunner
	var _ CommandRunner = &ExecRunner{}
}

func TestCheckResultFields(t *testing.T) {
	result := CheckResult{
		Name:   "test-check",
		Passed: true,
		Detail: "some detail",
		Error:  "",
	}
	assert.Equal(t, "test-check", result.Name)
	assert.True(t, result.Passed)
	assert.Equal(t, "some detail", result.Detail)
	assert.Empty(t, result.Error)
}

func TestGPUInfoFields(t *testing.T) {
	gpu := GPUInfo{
		Name:          "gfx1100",
		MarketingName: "AMD Radeon RX 7900 XTX",
		VRAMSizeKB:    25149440,
	}
	assert.Equal(t, "gfx1100", gpu.Name)
	assert.Equal(t, "AMD Radeon RX 7900 XTX", gpu.MarketingName)
	assert.Equal(t, uint64(25149440), gpu.VRAMSizeKB)
}
