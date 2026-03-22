// Package sandbox enforces filesystem and command execution restrictions.
// Writes are restricted to a working directory using os.Root (traversal-resistant).
// Reads are validated against an allowlist of path prefixes.
// Commands are validated against a Go toolchain allowlist.
package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sandbox constrains file operations to the working directory (writes)
// and an allowlist of paths (reads).
type Sandbox struct {
	cwd       string
	root      *os.Root
	readAllow []string // absolute path prefixes allowed for reads
}

// BlockedFile describes a file write that was blocked by the sandbox.
type BlockedFile struct {
	Path   string
	Reason string
}

// New creates a Sandbox rooted at the given working directory.
// The read allowlist is set to [cwd, "/etc", "/proc"].
func New(cwd string) (*Sandbox, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve cwd: %w", err)
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, fmt.Errorf("open root: %w", err)
	}
	return &Sandbox{
		cwd:       abs,
		root:      root,
		readAllow: []string{abs, "/etc", "/proc"},
	}, nil
}

// Close releases the os.Root file descriptor.
func (s *Sandbox) Close() error {
	return s.root.Close()
}

// CWD returns the resolved absolute working directory path.
func (s *Sandbox) CWD() string {
	return s.cwd
}

// WriteFile writes data to a path relative to the sandbox root.
// Absolute paths are rejected with a clear error. Path traversal
// (../) and symlink escapes are blocked by os.Root at the OS level.
func (s *Sandbox) WriteFile(name string, data []byte, perm os.FileMode) error {
	if filepath.IsAbs(name) {
		return fmt.Errorf("blocked: absolute path %s", name)
	}
	return s.root.WriteFile(name, data, perm)
}

// MkdirAll creates directories relative to the sandbox root.
// Absolute paths are rejected. Traversal is blocked by os.Root.
func (s *Sandbox) MkdirAll(name string, perm os.FileMode) error {
	if filepath.IsAbs(name) {
		return fmt.Errorf("blocked: absolute path %s", name)
	}
	return s.root.MkdirAll(name, perm)
}

// Stat returns FileInfo for a path relative to the sandbox root.
func (s *Sandbox) Stat(name string) (os.FileInfo, error) {
	return s.root.Stat(name)
}

// ValidateRead checks if a path is within the read allowlist.
// It resolves symlinks and relative paths before checking.
func (s *Sandbox) ValidateRead(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("blocked: cannot resolve %s", path)
	}
	// Resolve symlinks; fall back to unresolved if file doesn't exist
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		resolved = abs
	}
	for _, prefix := range s.readAllow {
		if resolved == prefix || strings.HasPrefix(resolved, prefix+"/") {
			return nil
		}
	}
	return fmt.Errorf("blocked: %s outside allowlist", path)
}

// allowedGoSubcommands lists the permitted go subcommands.
var allowedGoSubcommands = map[string]bool{
	"test":  true,
	"build": true,
	"vet":   true,
	"fmt":   true,
	"mod":   true, // further validated: only "mod tidy"
}

// allowedBuildCommands lists non-Go build tools that are permitted.
var allowedBuildCommands = map[string]bool{
	"cargo":  true,
	"rustc":  true,
	"make":   true,
	"npm":    true,
	"npx":    true,
	"python": true,
	"python3": true,
	"pip":    true,
	"gcc":    true,
	"g++":    true,
	"javac":  true,
	"mvn":    true,
	"gradle": true,
	"cmake":  true,
	"echo":   true,
	"bash":   true,
	"sh":     true,
}

// ValidateCommand checks if a command is in the allowlist.
// Allows go subcommands (test, build, vet, fmt, mod tidy) and common build tools.
// For go test, all non-flag package arguments must start with "./".
func ValidateCommand(name string, args []string) error {
	// Check non-Go build commands first
	if allowedBuildCommands[name] {
		return nil
	}

	if name != "go" {
		return fmt.Errorf("blocked: command %q not allowed", name)
	}
	if len(args) == 0 {
		return fmt.Errorf("blocked: bare 'go' command not allowed")
	}
	subcmd := args[0]
	if !allowedGoSubcommands[subcmd] {
		return fmt.Errorf("blocked: 'go %s' not allowed", subcmd)
	}
	// Special case: "go mod" only allows "tidy"
	if subcmd == "mod" {
		if len(args) < 2 || args[1] != "tidy" {
			return fmt.Errorf("blocked: only 'go mod tidy' is allowed")
		}
	}
	// Special case: "go test" packages must start with "./"
	if subcmd == "test" {
		for _, arg := range args[1:] {
			if strings.HasPrefix(arg, "-") {
				continue // flags are ok
			}
			if !strings.HasPrefix(arg, "./") {
				return fmt.Errorf("blocked: go test package %q must use ./... prefix", arg)
			}
		}
	}
	return nil
}
