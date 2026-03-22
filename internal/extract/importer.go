package extract

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
)

// sourceImporter implements types.Importer for project-local cross-package
// type resolution. Packages within the module are parsed from source; stdlib
// and external packages are delegated to the default importer.
type sourceImporter struct {
	fset       *token.FileSet
	packages   map[string]*types.Package // cache
	inProgress map[string]bool           // cycle detection
	modRoot    string
	modPath    string
	errors     []string // collected non-fatal errors
}

// newSourceImporter creates a sourceImporter for the given module.
func newSourceImporter(fset *token.FileSet, modRoot, modPath string) *sourceImporter {
	return &sourceImporter{
		fset:       fset,
		packages:   make(map[string]*types.Package),
		inProgress: make(map[string]bool),
		modRoot:    modRoot,
		modPath:    modPath,
	}
}

// Import satisfies the types.Importer interface.
func (si *sourceImporter) Import(path string) (*types.Package, error) {
	// Return cached package if available
	if pkg, ok := si.packages[path]; ok {
		return pkg, nil
	}

	// Cycle detection
	if si.inProgress[path] {
		return nil, fmt.Errorf("import cycle: %s", path)
	}

	// Non-module packages go to the default importer (stdlib, external)
	if !strings.HasPrefix(path, si.modPath) {
		return importer.Default().Import(path)
	}

	// Project-local package: parse from source
	si.inProgress[path] = true
	defer func() { delete(si.inProgress, path) }()

	// Compute directory: trim module prefix to get relative path
	relPath := strings.TrimPrefix(path, si.modPath)
	relPath = strings.TrimPrefix(relPath, "/")
	dir := si.modRoot
	if relPath != "" {
		dir = filepath.Join(si.modRoot, relPath)
	}

	// Verify directory exists
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("package dir %s: %w", dir, err)
	}

	// Parse Go files (exclude test files)
	filter := func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}
	pkgs, err := parser.ParseDir(si.fset, dir, filter, parser.ParseComments)
	if err != nil {
		si.errors = append(si.errors, fmt.Sprintf("// ERROR: parse %s: %v", path, err))
		return nil, err
	}

	// Collect AST files from the first non-test package
	var astFiles []*ast.File
	for pkgName, pkg := range pkgs {
		if strings.HasSuffix(pkgName, "_test") {
			continue
		}
		for _, f := range pkg.Files {
			astFiles = append(astFiles, f)
		}
		break
	}

	if len(astFiles) == 0 {
		return nil, fmt.Errorf("no Go files in %s", dir)
	}

	// Type-check with error collection (non-fatal)
	conf := types.Config{
		Importer: si,
		Error: func(err error) {
			si.errors = append(si.errors, fmt.Sprintf("// ERROR: %v", err))
		},
	}

	typePkg, err := conf.Check(path, si.fset, astFiles, nil)
	if err != nil {
		// Type errors are non-fatal; we still cache the partial package
		if typePkg == nil {
			return nil, err
		}
	}

	si.packages[path] = typePkg
	return typePkg, nil
}
