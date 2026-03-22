package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Write Operations ---

func TestWriteFile_RelativePath_Succeeds(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	err = sb.WriteFile("foo.go", []byte("package foo"), 0o644)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "foo.go"))
	require.NoError(t, err)
	assert.Equal(t, "package foo", string(data))
}

func TestWriteFile_AbsolutePath_Blocked(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	err = sb.WriteFile("/tmp/evil.go", []byte("bad"), 0o644)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
	assert.Contains(t, err.Error(), "absolute path")
}

func TestWriteFile_Traversal_Blocked(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	err = sb.WriteFile("../../etc/passwd", []byte("bad"), 0o644)
	require.Error(t, err)
	// os.Root rejects .. traversal
}

func TestWriteFile_SymlinkOutsideCWD_Blocked(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()

	// Create symlink inside CWD pointing outside
	link := filepath.Join(dir, "escape")
	err := os.Symlink(outside, link)
	require.NoError(t, err)

	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	err = sb.WriteFile("escape/evil.go", []byte("bad"), 0o644)
	require.Error(t, err)
	// os.Root blocks symlink traversal
}

// --- MkdirAll ---

func TestMkdirAll_RelativePath_Succeeds(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	err = sb.MkdirAll("sub/nested", 0o755)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dir, "sub", "nested"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestMkdirAll_AbsolutePath_Blocked(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	err = sb.MkdirAll("/tmp/dir", 0o755)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
	assert.Contains(t, err.Error(), "absolute path")
}

// --- WriteFile after MkdirAll ---

func TestWriteFile_NestedDir_Succeeds(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	err = sb.MkdirAll("sub/dir", 0o755)
	require.NoError(t, err)

	err = sb.WriteFile("sub/dir/foo.go", []byte("package sub"), 0o644)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "sub", "dir", "foo.go"))
	require.NoError(t, err)
	assert.Equal(t, "package sub", string(data))
}

// --- Stat ---

func TestStat_ExistingFile_ReturnsInfo(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "existing.go"), []byte("hello"), 0o644)
	require.NoError(t, err)

	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	info, err := sb.Stat("existing.go")
	require.NoError(t, err)
	assert.Equal(t, "existing.go", info.Name())
}

// --- Close ---

func TestClose_ReleasesRoot(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir)
	require.NoError(t, err)

	err = sb.Close()
	require.NoError(t, err)
}

// --- CWD ---

func TestCWD_ReturnsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	assert.True(t, filepath.IsAbs(sb.CWD()))
}

// --- ValidateRead ---

func TestValidateRead_InsideCWD_Allowed(t *testing.T) {
	dir := t.TempDir()
	// Create a file so EvalSymlinks can resolve it
	f := filepath.Join(dir, "file.go")
	err := os.WriteFile(f, []byte("x"), 0o644)
	require.NoError(t, err)

	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	err = sb.ValidateRead(f)
	assert.NoError(t, err)
}

func TestValidateRead_EtcHostname_Allowed(t *testing.T) {
	sb, err := New(t.TempDir())
	require.NoError(t, err)
	defer sb.Close()

	err = sb.ValidateRead("/etc/hostname")
	assert.NoError(t, err)
}

func TestValidateRead_ProcCpuinfo_Allowed(t *testing.T) {
	sb, err := New(t.TempDir())
	require.NoError(t, err)
	defer sb.Close()

	err = sb.ValidateRead("/proc/cpuinfo")
	assert.NoError(t, err)
}

func TestValidateRead_OutsideAllowlist_Blocked(t *testing.T) {
	sb, err := New(t.TempDir())
	require.NoError(t, err)
	defer sb.Close()

	err = sb.ValidateRead("/home/other/file.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateRead_Tmp_Blocked(t *testing.T) {
	sb, err := New(t.TempDir())
	require.NoError(t, err)
	defer sb.Close()

	err = sb.ValidateRead("/tmp/file")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateRead_SymlinkOutsideAllowlist_Blocked(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	err := os.WriteFile(outsideFile, []byte("secret"), 0o644)
	require.NoError(t, err)

	// Create symlink inside CWD pointing to outside file
	link := filepath.Join(dir, "sneaky.txt")
	err = os.Symlink(outsideFile, link)
	require.NoError(t, err)

	sb, err := New(dir)
	require.NoError(t, err)
	defer sb.Close()

	err = sb.ValidateRead(link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

// --- ValidateCommand ---

func TestValidateCommand_GoTest_Allowed(t *testing.T) {
	err := ValidateCommand("go", []string{"test", "-v", "./..."})
	assert.NoError(t, err)
}

func TestValidateCommand_GoBuild_Allowed(t *testing.T) {
	err := ValidateCommand("go", []string{"build", "./..."})
	assert.NoError(t, err)
}

func TestValidateCommand_GoVet_Allowed(t *testing.T) {
	err := ValidateCommand("go", []string{"vet", "./..."})
	assert.NoError(t, err)
}

func TestValidateCommand_GoFmt_Allowed(t *testing.T) {
	err := ValidateCommand("go", []string{"fmt", "./..."})
	assert.NoError(t, err)
}

func TestValidateCommand_GoModTidy_Allowed(t *testing.T) {
	err := ValidateCommand("go", []string{"mod", "tidy"})
	assert.NoError(t, err)
}

func TestValidateCommand_GoModDownload_Blocked(t *testing.T) {
	err := ValidateCommand("go", []string{"mod", "download"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateCommand_GoRun_Blocked(t *testing.T) {
	err := ValidateCommand("go", []string{"run", "."})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateCommand_NonGoCommand_Blocked(t *testing.T) {
	err := ValidateCommand("rm", []string{"-rf", "/"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateCommand_BareGo_Blocked(t *testing.T) {
	err := ValidateCommand("go", []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateCommand_GoTestAbsolutePath_Blocked(t *testing.T) {
	err := ValidateCommand("go", []string{"test", "/absolute/path"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateCommand_GoTestWithFlags_Allowed(t *testing.T) {
	err := ValidateCommand("go", []string{"test", "-v", "-count=1", "./internal/..."})
	assert.NoError(t, err)
}

func TestValidateCommand_GoTestExternalPkg_Blocked(t *testing.T) {
	err := ValidateCommand("go", []string{"test", "github.com/other/pkg"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

// --- BlockedFile ---

func TestBlockedFile_HasFields(t *testing.T) {
	b := BlockedFile{Path: "/tmp/evil.go", Reason: "outside CWD"}
	assert.Equal(t, "/tmp/evil.go", b.Path)
	assert.Equal(t, "outside CWD", b.Reason)
}
