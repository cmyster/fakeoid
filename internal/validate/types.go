package validate

import "os/exec"

// CommandRunner abstracts command execution for testability.
// Tests inject a mock; production code uses ExecRunner.
type CommandRunner interface {
	LookPath(file string) (string, error)
	Run(name string, args ...string) ([]byte, error)
}

// ExecRunner implements CommandRunner using real os/exec calls.
type ExecRunner struct{}

func (r *ExecRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (r *ExecRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// CheckResult holds the outcome of a single validation check.
type CheckResult struct {
	Name   string
	Passed bool
	Detail string
	Error  string
}

// GPUInfo holds information about a detected AMD GPU.
type GPUInfo struct {
	Name          string
	MarketingName string
	VRAMSizeKB    uint64
}
