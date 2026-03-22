package validate

// validationState carries data between sequential checks.
type validationState struct {
	llamaServerPath string
}

// RunAll executes all validation checks in sequence with fail-fast behavior.
// If any check fails, execution stops and results up to that point are returned.
func RunAll(runner CommandRunner, verbose bool) ([]CheckResult, error) {
	state := &validationState{}
	var results []CheckResult

	// Check 1: llama-server in PATH
	llamaResult, err := CheckLlamaServer(runner, verbose)
	if err != nil {
		return results, err
	}
	results = append(results, *llamaResult)
	if !llamaResult.Passed {
		return results, nil
	}
	// Extract llama-server path from Detail for next check.
	// When not verbose, Detail is empty, so we re-resolve the path.
	path, _ := runner.LookPath("llama-server")
	state.llamaServerPath = path

	// Check 2: ROCm build (ldd check on llama-server)
	rocmBuildResult, err := CheckROCmBuild(runner, state.llamaServerPath, verbose)
	if err != nil {
		return results, err
	}
	results = append(results, *rocmBuildResult)
	if !rocmBuildResult.Passed {
		return results, nil
	}

	// Check 3: rocminfo in PATH
	rocmInfoResult, err := CheckROCmInfo(runner, verbose)
	if err != nil {
		return results, err
	}
	results = append(results, *rocmInfoResult)
	if !rocmInfoResult.Passed {
		return results, nil
	}

	// Check 4: GPU detection
	gpuResult, _, err := CheckGPU(runner, verbose)
	if err != nil {
		return results, err
	}
	results = append(results, *gpuResult)
	if !gpuResult.Passed {
		return results, nil
	}

	return results, nil
}
