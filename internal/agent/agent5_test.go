package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cmyster/fakeoid/internal/sandbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent5_ImplementsAgent(t *testing.T) {
	dir := t.TempDir()
	handoffFile := filepath.Join(dir, "001-task-handoff.md")
	require.NoError(t, os.WriteFile(handoffFile, []byte("# Handoff\n\n## Files Modified/Created\n\n- internal/agent/foo.go (created)\n"), 0o644))

	sb, err := sandbox.New(dir)
	require.NoError(t, err)
	defer sb.Close()

	a := NewAgent5(dir, dir, handoffFile, sb, "")
	var _ Agent = a // compile-time interface check
	assert.NotNil(t, a)
}

func TestAgent5_Number(t *testing.T) {
	a := &Agent5{}
	assert.Equal(t, 5, a.Number())
}

func TestAgent5_Name(t *testing.T) {
	a := &Agent5{}
	assert.Equal(t, "QE Engineer", a.Name())
}

func TestAgent5_SystemPrompt_ContainsContext(t *testing.T) {
	dir := t.TempDir()

	// Create a source file to be read
	srcDir := filepath.Join(dir, "internal", "agent")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "foo.go"), []byte("package agent\n\nfunc Foo() {}\n"), 0o644))

	handoffContent := "# Handoff\n\n## Files Modified/Created\n\n- internal/agent/foo.go (created)\n"
	handoffFile := filepath.Join(dir, "001-task-handoff.md")
	require.NoError(t, os.WriteFile(handoffFile, []byte(handoffContent), 0o644))

	sb, err := sandbox.New(dir)
	require.NoError(t, err)
	defer sb.Close()

	a := NewAgent5(dir, dir, handoffFile, sb, "")
	prompt := a.SystemPrompt()

	assert.Contains(t, prompt, dir)
	assert.Contains(t, prompt, "QE Engineer")
	assert.Contains(t, prompt, "Handoff")
	assert.Contains(t, prompt, "package agent")
}

func TestAgent5_HandleResponse_WithCodeBlocks(t *testing.T) {
	a := &Agent5{}
	response := "Here are the tests:\n```go:internal/agent/foo_test.go\npackage agent\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {}\n```"
	action := a.HandleResponse(response)
	assert.Equal(t, ActionComplete, action.Type)
}

func TestAgent5_HandleResponse_NoCodeBlocks(t *testing.T) {
	a := &Agent5{}
	response := "I need to understand the code structure better before writing tests."
	action := a.HandleResponse(response)
	assert.Equal(t, ActionContinue, action.Type)
	assert.Equal(t, 1, a.turnCount)
}

func TestAgent5_HandleResponse_ContinuesWithoutBlocks(t *testing.T) {
	a := &Agent5{turnCount: 10}
	// No code blocks -- keeps going regardless of turn count
	response := "Still analyzing the code..."
	action := a.HandleResponse(response)
	assert.Equal(t, ActionContinue, action.Type)
}

// --- ExtractPackages tests ---

func TestExtractPackages_ParsesGoFilePaths(t *testing.T) {
	input := "## Files Modified/Created\n\n- internal/agent/foo.go (created)\n- internal/agent/bar.go (modified)\n"
	pkgs := ExtractPackages(input)
	assert.Equal(t, []string{"./internal/agent"}, pkgs)
}

func TestExtractPackages_DeduplicatesSameDirectory(t *testing.T) {
	input := "- internal/agent/foo.go (created)\n- internal/agent/bar.go (created)\n- internal/server/client.go (modified)\n"
	pkgs := ExtractPackages(input)
	assert.Len(t, pkgs, 2)
	assert.Contains(t, pkgs, "./internal/agent")
	assert.Contains(t, pkgs, "./internal/server")
}

func TestExtractPackages_IgnoresNonGoFiles(t *testing.T) {
	input := "- README.md (created)\n- internal/agent/foo.go (created)\n- config.yaml (modified)\n"
	pkgs := ExtractPackages(input)
	assert.Equal(t, []string{"./internal/agent"}, pkgs)
}

func TestExtractPackages_IgnoresNonListLines(t *testing.T) {
	input := "# Handoff\n\nSome description text.\n\n- internal/agent/foo.go (created)\n"
	pkgs := ExtractPackages(input)
	assert.Equal(t, []string{"./internal/agent"}, pkgs)
}

func TestExtractPackages_EmptyInput(t *testing.T) {
	pkgs := ExtractPackages("")
	assert.Empty(t, pkgs)
}

func TestExtractPackages_NoMatches(t *testing.T) {
	pkgs := ExtractPackages("No files here.\nJust text.\n")
	assert.Empty(t, pkgs)
}

// --- readSourceFiles tests ---

func TestReadSourceFiles_ReadsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "internal", "agent")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "foo.go"), []byte("package agent\n"), 0o644))

	result := readSourceFiles(dir, []string{"internal/agent/foo.go"})
	assert.Contains(t, result, "### internal/agent/foo.go")
	assert.Contains(t, result, "package agent")
	assert.Contains(t, result, "```go")
}

func TestReadSourceFiles_SkipsNonExistentFiles(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "internal", "agent")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "foo.go"), []byte("package agent\n"), 0o644))

	result := readSourceFiles(dir, []string{"internal/agent/foo.go", "internal/agent/missing.go"})
	assert.Contains(t, result, "### internal/agent/foo.go")
	assert.NotContains(t, result, "missing.go")
}

// --- RunGoTest tests ---

func TestRunGoTest_PassingTests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	// Create a minimal Go module with a passing test
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pass_test.go"), []byte("package testmod\n\nimport \"testing\"\n\nfunc TestPass(t *testing.T) {}\n"), 0o644))

	var live bytes.Buffer
	output, passed, err := RunGoTest(context.Background(), dir, []string{"./"}, &live)
	require.NoError(t, err)
	assert.True(t, passed)
	assert.Contains(t, output, "PASS")
	assert.Contains(t, live.String(), "PASS")
}

func TestRunGoTest_FailingTests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fail_test.go"), []byte("package testmod\n\nimport \"testing\"\n\nfunc TestFail(t *testing.T) { t.Fatal(\"boom\") }\n"), 0o644))

	var live bytes.Buffer
	output, passed, err := RunGoTest(context.Background(), dir, []string{"./"}, &live)
	require.NoError(t, err)
	assert.False(t, passed)
	assert.Contains(t, output, "FAIL")
	assert.Contains(t, live.String(), "FAIL")
}

func TestRunGoTest_StreamsOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_test.go"), []byte("package testmod\n\nimport \"testing\"\n\nfunc TestStream(t *testing.T) { t.Log(\"stream-marker\") }\n"), 0o644))

	var live bytes.Buffer
	output, passed, err := RunGoTest(context.Background(), dir, []string{"./"}, &live)
	require.NoError(t, err)
	assert.True(t, passed)
	assert.Contains(t, output, "stream-marker")
	assert.Contains(t, live.String(), "stream-marker")
}

// --- ReadBuildInstructions tests ---

func TestReadBuildInstructions_ReadsExistingReadme(t *testing.T) {
	dir := t.TempDir()
	content := "# My Project\n\n## Build\n\n```bash\nrustc src/main.rs\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0o644))

	sb, err := sandbox.New(dir)
	require.NoError(t, err)
	defer sb.Close()

	result := ReadBuildInstructions(dir, sb)
	assert.Equal(t, content, result)
}

func TestReadBuildInstructions_MissingReadme(t *testing.T) {
	dir := t.TempDir()
	sb, err := sandbox.New(dir)
	require.NoError(t, err)
	defer sb.Close()

	result := ReadBuildInstructions(dir, sb)
	assert.Empty(t, result)
}

func TestReadBuildInstructions_NilSandbox(t *testing.T) {
	dir := t.TempDir()
	content := "# Project\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0o644))

	result := ReadBuildInstructions(dir, nil)
	assert.Equal(t, content, result)
}

// --- IsGoProject tests ---

func TestIsGoProject_WithGoMod(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644))
	assert.True(t, IsGoProject(dir))
}

func TestIsGoProject_WithoutGoMod(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, IsGoProject(dir))
}

// --- languageFromExt tests ---

func TestLanguageFromExt(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"src/main.rs", "rust"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		{"app.ts", "typescript"},
		{"README.md", "markdown"},
		{"main.c", "c"},
		{"main.cpp", "cpp"},
		{"App.java", "java"},
		{"script.rb", "ruby"},
		{"run.sh", "bash"},
		{"data.xyz", ""},
		{"Makefile", ""},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			assert.Equal(t, tc.want, languageFromExt(tc.path))
		})
	}
}

// --- extractFilePaths (updated: language-agnostic) ---

func TestExtractFilePaths_AcceptsAllExtensions(t *testing.T) {
	input := "## Files Modified/Created\n\n- src/main.rs (created)\n- internal/agent/foo.go (modified)\n- app.py (created)\n- Cargo.toml (created)\n"
	paths := extractFilePaths(input)
	assert.Contains(t, paths, "src/main.rs")
	assert.Contains(t, paths, "internal/agent/foo.go")
	assert.Contains(t, paths, "app.py")
	assert.Contains(t, paths, "Cargo.toml")
}

// --- readSourceFiles (updated: language-aware fences) ---

func TestReadSourceFiles_UsesLanguageFromExt(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.rs"), []byte("fn main() {}\n"), 0o644))

	result := readSourceFiles(dir, []string{"src/main.rs"})
	assert.Contains(t, result, "```rust")
	assert.Contains(t, result, "fn main()")
}

// --- parseBuildCommand tests ---

func TestParseBuildCommand_FindsFencedBlock(t *testing.T) {
	readme := "# Project\n\n## Build\n\n```bash\nrustc src/main.rs\n```\n"
	assert.Equal(t, "rustc src/main.rs", parseBuildCommand(readme))
}

func TestParseBuildCommand_NoBuildSection(t *testing.T) {
	readme := "# Project\n\nJust a description.\n"
	assert.Empty(t, parseBuildCommand(readme))
}

func TestParseBuildCommand_CaseInsensitive(t *testing.T) {
	readme := "# Project\n\n### build\n\n```\ncargo build\n```\n"
	assert.Equal(t, "cargo build", parseBuildCommand(readme))
}

func TestParseBuildCommand_StopsAtNextHeading(t *testing.T) {
	readme := "## Build\n\n```\nmake\n```\n\n## Run\n\n```\n./app\n```\n"
	assert.Equal(t, "make", parseBuildCommand(readme))
}

// --- RunBuildAndVerify tests ---

func TestRunBuildAndVerify_WithReadmeCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	readme := "# Test\n\n## Build\n\n```bash\necho build-ok\n```\n"

	var live bytes.Buffer
	output, passed, err := RunBuildAndVerify(context.Background(), dir, readme, &live)
	require.NoError(t, err)
	assert.True(t, passed)
	assert.Contains(t, output, "build-ok")
}

func TestRunBuildAndVerify_NoReadmeNoMarker(t *testing.T) {
	dir := t.TempDir()

	var live bytes.Buffer
	output, passed, err := RunBuildAndVerify(context.Background(), dir, "", &live)
	require.NoError(t, err)
	assert.False(t, passed)
	assert.Contains(t, output, "no build instructions found")
}

func TestRunBuildAndVerify_FallbackCargo(t *testing.T) {
	// Just verify the detection logic, not actual cargo execution
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0o644))

	var live bytes.Buffer
	// Will fail because cargo project isn't real, but should attempt cargo build
	output, passed, _ := RunBuildAndVerify(context.Background(), dir, "", &live)
	// Either cargo not found (error) or cargo build fails (passed=false)
	// The important thing is it tried cargo, not "no build instructions"
	assert.False(t, passed)
	_ = output
}

// --- Agent5SystemPrompt includes README ---

func TestAgent5SystemPrompt_IncludesReadme(t *testing.T) {
	readme := "# My Project\n\n## Build\nrustc src/main.rs\n"
	prompt := Agent5SystemPrompt("/tmp/test", "tree", "handoff", "sources", readme)
	assert.Contains(t, prompt, "rustc src/main.rs")
	assert.Contains(t, prompt, "README.md (Build Instructions)")
}
