package validate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckROCmInfo_Found(t *testing.T) {
	runner := &mockRunner{
		lookPathFunc: func(file string) (string, error) {
			assert.Equal(t, "rocminfo", file)
			return "/usr/bin/rocminfo", nil
		},
	}

	result, err := CheckROCmInfo(runner, true)
	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Equal(t, "rocminfo", result.Name)
}

func TestCheckROCmInfo_NotFound(t *testing.T) {
	runner := &mockRunner{
		lookPathFunc: func(file string) (string, error) {
			return "", errors.New("not found")
		},
	}

	result, err := CheckROCmInfo(runner, false)
	require.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Equal(t, "rocminfo not found in PATH", result.Error)
}

// Realistic rocminfo output based on actual system (AMD Ryzen 9 5900X + RX 7900 XTX).
const rocmInfoGPUOutput = `ROCk module is loaded
=====================
HSA System Attributes
=====================
Runtime Version:         1.1
System Timestamp Freq.:  1000.000000MHz

==========
HSA Agents
==========
*******
Agent 1
*******
  Name:                    AMD Ryzen 9 5900X 12-Core Processor
  Marketing Name:          AMD Ryzen 9 5900X 12-Core Processor
  Vendor Name:             CPU
  Feature:                 None specified
  Compute Unit:            24
  Device Type:             CPU
  Pool Info:
    Pool 1
      Segment:                 GLOBAL; FLAGS: FINE GRAINED
      Size:                    33521508(0x1fff764) KB
*******
Agent 2
*******
  Name:                    gfx1100
  Marketing Name:          AMD Radeon RX 7900 XTX
  Vendor Name:             AMD
  Feature:                 KERNEL_DISPATCH
  Compute Unit:            96
  Device Type:             GPU
  Pool Info:
    Pool 1
      Segment:                 GLOBAL; FLAGS: COARSE GRAINED
      Size:                    25149440(0x17fc000) KB
    Pool 2
      Segment:                 GLOBAL; FLAGS: FINE GRAINED
      Size:                    33521508(0x1fff764) KB
*** Done ***
`

func TestCheckGPU_Found(t *testing.T) {
	runner := &mockRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			assert.Equal(t, "rocminfo", name)
			return []byte(rocmInfoGPUOutput), nil
		},
	}

	result, gpus, err := CheckGPU(runner, true)
	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Equal(t, "gpu", result.Name)
	require.Len(t, gpus, 1)
	assert.Equal(t, "gfx1100", gpus[0].Name)
	assert.Equal(t, "AMD Radeon RX 7900 XTX", gpus[0].MarketingName)
	assert.Equal(t, 96, gpus[0].ComputeUnits)
	assert.Contains(t, result.Detail, "gfx1100")
}

const rocmInfoCPUOnly = `ROCk module is loaded
=====================
HSA System Attributes
=====================
Runtime Version:         1.1

==========
HSA Agents
==========
*******
Agent 1
*******
  Name:                    AMD Ryzen 9 5900X 12-Core Processor
  Marketing Name:          AMD Ryzen 9 5900X 12-Core Processor
  Vendor Name:             CPU
  Feature:                 None specified
  Device Type:             CPU
  Pool Info:
    Pool 1
      Segment:                 GLOBAL; FLAGS: FINE GRAINED
      Size:                    33521508(0x1fff764) KB
*** Done ***
`

func TestCheckGPU_NoneFound(t *testing.T) {
	runner := &mockRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return []byte(rocmInfoCPUOnly), nil
		},
	}

	result, gpus, err := CheckGPU(runner, false)
	require.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Equal(t, "no AMD GPU detected", result.Error)
	assert.Empty(t, gpus)
}

const rocmInfoMultiAgent = `ROCk module is loaded
==========
HSA Agents
==========
*******
Agent 1
*******
  Name:                    AMD Ryzen 9 5900X 12-Core Processor
  Marketing Name:          AMD Ryzen 9 5900X 12-Core Processor
  Compute Unit:            24
  Device Type:             CPU
  Pool Info:
    Pool 1
      Segment:                 GLOBAL; FLAGS: FINE GRAINED
      Size:                    33521508(0x1fff764) KB
*******
Agent 2
*******
  Name:                    gfx1100
  Marketing Name:          AMD Radeon RX 7900 XTX
  Compute Unit:            96
  Device Type:             GPU
  Pool Info:
    Pool 1
      Segment:                 GLOBAL; FLAGS: COARSE GRAINED
      Size:                    25149440(0x17fc000) KB
*******
Agent 3
*******
  Name:                    gfx1030
  Marketing Name:          AMD Radeon RX 6800 XT
  Compute Unit:            72
  Device Type:             GPU
  Pool Info:
    Pool 1
      Segment:                 GLOBAL; FLAGS: COARSE GRAINED
      Size:                    16777216(0x1000000) KB
*** Done ***
`

func TestCheckGPU_MultipleAgents(t *testing.T) {
	runner := &mockRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return []byte(rocmInfoMultiAgent), nil
		},
	}

	result, gpus, err := CheckGPU(runner, true)
	require.NoError(t, err)
	assert.True(t, result.Passed)
	// Only GPU agents returned, not CPU
	require.Len(t, gpus, 2)
	assert.Equal(t, "gfx1100", gpus[0].Name)
	assert.Equal(t, 96, gpus[0].ComputeUnits)
	assert.Equal(t, "gfx1030", gpus[1].Name)
	assert.Equal(t, 72, gpus[1].ComputeUnits)
}

func TestCheckGPU_VRAMExtraction(t *testing.T) {
	runner := &mockRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return []byte(rocmInfoGPUOutput), nil
		},
	}

	_, gpus, err := CheckGPU(runner, false)
	require.NoError(t, err)
	require.Len(t, gpus, 1)
	// COARSE GRAINED pool size = 25149440 KB
	assert.Equal(t, uint64(25149440), gpus[0].VRAMSizeKB)
}

func TestCheckGPU_RocminfoError(t *testing.T) {
	runner := &mockRunner{
		runFunc: func(name string, args ...string) ([]byte, error) {
			return nil, errors.New("rocminfo failed")
		},
	}

	result, _, err := CheckGPU(runner, false)
	require.NoError(t, err)
	assert.False(t, result.Passed)
	assert.Contains(t, result.Error, "failed to run rocminfo")
}
