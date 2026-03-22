package extract

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
)

// GoExtractor implements the Extractor interface for Go source code using go/ast.
type GoExtractor struct{}

// NewGoExtractor returns a new GoExtractor.
func NewGoExtractor() *GoExtractor {
	return &GoExtractor{}
}

// Extract parses Go source files under root and returns structured extraction data.
func (g *GoExtractor) Extract(root string) (*ProjectInfo, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}

	info := &ProjectInfo{
		ImportGraph: make(map[string][]string),
	}

	// Try to detect module path from go.mod
	modRoot, modPath := findModuleRoot(absRoot)
	info.ModulePath = modPath

	// Collect package directories
	pkgDirs, errs := collectPackageDirs(absRoot)
	info.Errors = append(info.Errors, errs...)

	fset := token.NewFileSet()

	for _, dir := range pkgDirs {
		pkg, pkgErrs := extractPackage(fset, dir, absRoot, modRoot, modPath)
		if pkg != nil {
			info.Packages = append(info.Packages, pkg)
			if pkg.ImportPath != "" && len(pkg.Imports) > 0 {
				info.ImportGraph[pkg.ImportPath] = pkg.Imports
			}
		}
		info.Errors = append(info.Errors, pkgErrs...)
	}

	// Pass 2: Type-check for interface satisfaction (only if we have a module)
	if modPath != "" {
		typeCheckErrs := g.typeCheckPass(fset, info, modRoot, modPath)
		info.Errors = append(info.Errors, typeCheckErrs...)
	}

	// Compute stats
	info.Stats = computeStats(info.Packages)

	return info, nil
}

// typeCheckPass runs go/types over all packages to detect interface satisfaction.
func (g *GoExtractor) typeCheckPass(fset *token.FileSet, info *ProjectInfo, modRoot, modPath string) []string {
	si := newSourceImporter(fset, modRoot, modPath)

	var typePkgs []*types.Package

	for _, pkg := range info.Packages {
		if pkg.ImportPath == "" {
			continue
		}

		// Type-check this package via the source importer
		typePkg, err := si.Import(pkg.ImportPath)
		if err != nil {
			// Already collected in si.errors; continue
			continue
		}
		typePkgs = append(typePkgs, typePkg)
	}

	// Find implementations across all type-checked packages
	impls := findImplementations(typePkgs)
	info.Implements = impls

	// Populate per-type Implements field
	populateTypeImplements(info, impls)

	return si.errors
}

// findImplementations checks all concrete types against all interfaces
// across all provided packages.
func findImplementations(pkgs []*types.Package) []InterfaceSatisfaction {
	type ifaceEntry struct {
		pkgPath string
		name    string
		iface   *types.Interface
	}
	type concreteEntry struct {
		pkgPath string
		name    string
		typ     types.Type
	}

	var ifaces []ifaceEntry
	var concretes []concreteEntry

	for _, pkg := range pkgs {
		scope := pkg.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			tn, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			underlying := tn.Type().Underlying()
			if iface, ok := underlying.(*types.Interface); ok {
				// Skip empty interfaces
				if iface.NumMethods() == 0 {
					continue
				}
				ifaces = append(ifaces, ifaceEntry{
					pkgPath: pkg.Path(),
					name:    name,
					iface:   iface,
				})
			} else {
				concretes = append(concretes, concreteEntry{
					pkgPath: pkg.Path(),
					name:    name,
					typ:     tn.Type(),
				})
			}
		}
	}

	var results []InterfaceSatisfaction
	for _, c := range concretes {
		for _, iface := range ifaces {
			// Skip self-implementation (same type/name in same package)
			if c.pkgPath == iface.pkgPath && c.name == iface.name {
				continue
			}
			// Check T implements I, or *T implements I
			if types.Implements(c.typ, iface.iface) || types.Implements(types.NewPointer(c.typ), iface.iface) {
				results = append(results, InterfaceSatisfaction{
					Type:      c.pkgPath + "." + c.name,
					Interface: iface.pkgPath + "." + iface.name,
				})
			}
		}
	}

	return results
}

// populateTypeImplements fills in each TypeInfo.Implements field based on the
// computed interface satisfactions.
func populateTypeImplements(info *ProjectInfo, impls []InterfaceSatisfaction) {
	// Build lookup: "pkgPath.TypeName" -> list of interface names
	implMap := make(map[string][]string)
	for _, impl := range impls {
		implMap[impl.Type] = append(implMap[impl.Type], impl.Interface)
	}

	for i, pkg := range info.Packages {
		for j, ty := range pkg.Types {
			key := pkg.ImportPath + "." + ty.Name
			if ifaces, ok := implMap[key]; ok {
				info.Packages[i].Types[j].Implements = ifaces
			}
		}
	}
}

// findModuleRoot walks up from start looking for go.mod.
// Returns the directory containing go.mod and the module path.
func findModuleRoot(start string) (root, modPath string) {
	dir := start
	for {
		gomod := filepath.Join(dir, "go.mod")
		data, err := os.ReadFile(gomod)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					modPath = strings.TrimSpace(strings.TrimPrefix(line, "module"))
					return dir, modPath
				}
			}
			return dir, ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return start, ""
}

// collectPackageDirs finds directories containing .go files under root,
// excluding vendor/, .git/, and testdata/ within child directories.
func collectPackageDirs(root string) ([]string, []string) {
	var dirs []string
	var errs []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, fmt.Sprintf("walk error: %s: %v", path, err))
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		name := d.Name()
		// Skip hidden dirs and vendor
		if name == ".git" || name == "vendor" {
			return filepath.SkipDir
		}
		// Skip testdata in subdirectories (not the root itself)
		if name == "testdata" && path != root {
			return filepath.SkipDir
		}

		// Check if directory has .go files (not test files)
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
				dirs = append(dirs, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		errs = append(errs, fmt.Sprintf("walk root: %v", err))
	}
	return dirs, errs
}

// goFileFilter returns a filter function that excludes test files.
func goFileFilter(fi os.FileInfo) bool {
	return !strings.HasSuffix(fi.Name(), "_test.go")
}

// isGeneratedFile checks if an AST file contains a "Code generated" + "DO NOT EDIT" comment.
func isGeneratedFile(file *ast.File) bool {
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := c.Text
			if strings.Contains(text, "Code generated") && strings.Contains(text, "DO NOT EDIT") {
				return true
			}
		}
	}
	return false
}

// extractPackage parses and extracts symbols from a single package directory.
func extractPackage(fset *token.FileSet, dir, scanRoot, modRoot, modPath string) (*PackageInfo, []string) {
	var errs []string

	pkgs, err := parser.ParseDir(fset, dir, goFileFilter, parser.ParseComments)
	if err != nil {
		return nil, []string{fmt.Sprintf("parse %s: %v", dir, err)}
	}

	for pkgName, pkg := range pkgs {
		// Skip test packages
		if strings.HasSuffix(pkgName, "_test") {
			continue
		}

		pkgInfo := &PackageInfo{
			Name: pkgName,
		}

		// Compute relative path and import path
		relPath, _ := filepath.Rel(scanRoot, dir)
		if relPath == "." {
			relPath = ""
		}
		pkgInfo.RelPath = relPath

		if modPath != "" && modRoot != "" {
			modRel, _ := filepath.Rel(modRoot, dir)
			if modRel == "." {
				pkgInfo.ImportPath = modPath
			} else {
				pkgInfo.ImportPath = modPath + "/" + filepath.ToSlash(modRel)
			}
		}

		importsSet := make(map[string]bool)
		var allFunctions []FunctionInfo
		typeMap := make(map[string]*TypeInfo)
		var typeOrder []string

		for fileName, file := range pkg.Files {
			// Skip generated files
			if isGeneratedFile(file) {
				continue
			}

			baseName := filepath.Base(fileName)
			pkgInfo.Files = append(pkgInfo.Files, baseName)

			// Extract package doc from first file that has it
			if pkgInfo.Doc == "" && file.Doc != nil {
				pkgInfo.Doc = strings.TrimSpace(file.Doc.Text())
			}

			// Extract build tags
			for _, cg := range file.Comments {
				for _, c := range cg.List {
					if strings.HasPrefix(c.Text, "//go:build ") {
						pkgInfo.BuildTags = append(pkgInfo.BuildTags, strings.TrimPrefix(c.Text, "//go:build "))
					}
				}
			}

			// Collect imports
			for _, imp := range file.Imports {
				impPath := strings.Trim(imp.Path.Value, `"`)
				importsSet[impPath] = true
			}

			// Process declarations
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					fi := extractFuncInfo(fset, d, baseName)
					allFunctions = append(allFunctions, fi)

				case *ast.GenDecl:
					switch d.Tok {
					case token.TYPE:
						for _, spec := range d.Specs {
							ts, ok := spec.(*ast.TypeSpec)
							if !ok {
								continue
							}
							ti := extractTypeInfo(fset, d, ts, baseName)
							typeMap[ti.Name] = &ti
							typeOrder = append(typeOrder, ti.Name)
						}

					case token.CONST:
						for _, spec := range d.Specs {
							vs, ok := spec.(*ast.ValueSpec)
							if !ok {
								continue
							}
							consts := extractConstInfo(fset, d, vs, baseName)
							pkgInfo.Constants = append(pkgInfo.Constants, consts...)
						}

					case token.VAR:
						for _, spec := range d.Specs {
							vs, ok := spec.(*ast.ValueSpec)
							if !ok {
								continue
							}
							vars := extractVarInfo(fset, d, vs, baseName)
							pkgInfo.Variables = append(pkgInfo.Variables, vars...)
						}
					}
				}
			}
		}

		// Deduplicate imports
		for imp := range importsSet {
			pkgInfo.Imports = append(pkgInfo.Imports, imp)
		}

		// Group methods by receiver type and separate init funcs
		for _, fn := range allFunctions {
			if fn.InitFunc {
				pkgInfo.InitFuncs = append(pkgInfo.InitFuncs, fn)
				continue
			}
			if fn.Receiver != "" {
				baseName := receiverBaseName(fn.Receiver)
				if ti, ok := typeMap[baseName]; ok {
					ti.Methods = append(ti.Methods, fn)
					continue
				}
			}
			pkgInfo.Functions = append(pkgInfo.Functions, fn)
		}

		// Build ordered type list
		for _, name := range typeOrder {
			if ti, ok := typeMap[name]; ok {
				pkgInfo.Types = append(pkgInfo.Types, *ti)
			}
		}

		return pkgInfo, errs
	}

	// If all files were generated/excluded, return empty package
	return nil, errs
}

// extractFuncInfo builds a FunctionInfo from a function declaration.
func extractFuncInfo(fset *token.FileSet, fn *ast.FuncDecl, fileName string) FunctionInfo {
	fi := FunctionInfo{
		Name:      fn.Name.Name,
		Signature: formatSignature(fset, fn),
		Source:    fmt.Sprintf("%s:%d", fileName, fset.Position(fn.Pos()).Line),
	}

	if fn.Doc != nil {
		fi.Doc = strings.TrimSpace(fn.Doc.Text())
	}

	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		fi.Receiver = exprToString(fn.Recv.List[0].Type)
	}

	if fn.Name.Name == "init" && fn.Recv == nil {
		fi.InitFunc = true
	}

	// Include body for short functions
	if fn.Body != nil {
		startLine := fset.Position(fn.Body.Pos()).Line
		endLine := fset.Position(fn.Body.End()).Line
		bodyLines := endLine - startLine
		if bodyLines <= 10 {
			fi.Body = nodeToString(fset, fn.Body)
		}
	}

	return fi
}

// extractTypeInfo builds a TypeInfo from a type spec.
func extractTypeInfo(fset *token.FileSet, gd *ast.GenDecl, ts *ast.TypeSpec, fileName string) TypeInfo {
	ti := TypeInfo{
		Name:   ts.Name.Name,
		Source: fmt.Sprintf("%s:%d", fileName, fset.Position(ts.Pos()).Line),
	}

	// Doc: prefer TypeSpec doc, fall back to GenDecl doc
	if ts.Doc != nil {
		ti.Doc = strings.TrimSpace(ts.Doc.Text())
	} else if gd.Doc != nil {
		ti.Doc = strings.TrimSpace(gd.Doc.Text())
	}

	// Determine kind
	switch t := ts.Type.(type) {
	case *ast.StructType:
		ti.Kind = TypeKindStruct
		ti.Fields = extractStructFields(t)
	case *ast.InterfaceType:
		ti.Kind = TypeKindInterface
	default:
		if ts.Assign.IsValid() {
			ti.Kind = TypeKindAlias
		} else {
			ti.Kind = TypeKindNamed
		}
	}

	return ti
}

// extractStructFields extracts fields from a struct type.
func extractStructFields(st *ast.StructType) []FieldInfo {
	var fields []FieldInfo
	if st.Fields == nil {
		return fields
	}

	for _, field := range st.Fields.List {
		fi := FieldInfo{
			Type: exprToString(field.Type),
		}

		if field.Doc != nil {
			fi.Doc = strings.TrimSpace(field.Doc.Text())
		}

		if field.Tag != nil {
			// Strip backticks from tag
			fi.Tag = strings.Trim(field.Tag.Value, "`")
		}

		if len(field.Names) == 0 {
			// Embedded field
			fi.Embedded = true
			fi.Name = baseTypeName(field.Type)
			fields = append(fields, fi)
		} else {
			for _, name := range field.Names {
				f := fi // copy
				f.Name = name.Name
				fields = append(fields, f)
			}
		}
	}
	return fields
}

// extractConstInfo extracts constant declarations.
func extractConstInfo(fset *token.FileSet, gd *ast.GenDecl, vs *ast.ValueSpec, fileName string) []ConstInfo {
	var consts []ConstInfo
	for i, name := range vs.Names {
		ci := ConstInfo{
			Name:   name.Name,
			Source: fmt.Sprintf("%s:%d", fileName, fset.Position(name.Pos()).Line),
		}

		// Doc: prefer ValueSpec doc, fall back to GenDecl doc (only if single spec)
		if vs.Doc != nil {
			ci.Doc = strings.TrimSpace(vs.Doc.Text())
		} else if gd.Doc != nil && len(gd.Specs) == 1 {
			ci.Doc = strings.TrimSpace(gd.Doc.Text())
		}

		if vs.Type != nil {
			ci.Type = exprToString(vs.Type)
		}
		if i < len(vs.Values) {
			ci.Value = nodeToString(fset, vs.Values[i])
		}
		consts = append(consts, ci)
	}
	return consts
}

// extractVarInfo extracts variable declarations.
func extractVarInfo(fset *token.FileSet, gd *ast.GenDecl, vs *ast.ValueSpec, fileName string) []VarInfo {
	var vars []VarInfo
	for _, name := range vs.Names {
		vi := VarInfo{
			Name:   name.Name,
			Source: fmt.Sprintf("%s:%d", fileName, fset.Position(name.Pos()).Line),
		}

		if vs.Doc != nil {
			vi.Doc = strings.TrimSpace(vs.Doc.Text())
		} else if gd.Doc != nil && len(gd.Specs) == 1 {
			vi.Doc = strings.TrimSpace(gd.Doc.Text())
		}

		if vs.Type != nil {
			vi.Type = exprToString(vs.Type)
		}
		vars = append(vars, vi)
	}
	return vars
}

// formatSignature builds a function signature string.
func formatSignature(fset *token.FileSet, fn *ast.FuncDecl) string {
	var buf bytes.Buffer
	buf.WriteString("func ")

	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		buf.WriteString("(")
		r := fn.Recv.List[0]
		if len(r.Names) > 0 {
			buf.WriteString(r.Names[0].Name)
			buf.WriteString(" ")
		}
		buf.WriteString(exprToString(r.Type))
		buf.WriteString(") ")
	}

	buf.WriteString(fn.Name.Name)
	buf.WriteString("(")

	if fn.Type.Params != nil {
		var params []string
		for _, p := range fn.Type.Params.List {
			typeStr := exprToString(p.Type)
			if len(p.Names) == 0 {
				params = append(params, typeStr)
			} else {
				for _, n := range p.Names {
					params = append(params, n.Name+" "+typeStr)
				}
			}
		}
		buf.WriteString(strings.Join(params, ", "))
	}

	buf.WriteString(")")

	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		results := fn.Type.Results.List
		if len(results) == 1 && len(results[0].Names) == 0 {
			buf.WriteString(" ")
			buf.WriteString(exprToString(results[0].Type))
		} else {
			buf.WriteString(" (")
			var rets []string
			for _, r := range results {
				typeStr := exprToString(r.Type)
				if len(r.Names) == 0 {
					rets = append(rets, typeStr)
				} else {
					for _, n := range r.Names {
						rets = append(rets, n.Name+" "+typeStr)
					}
				}
			}
			buf.WriteString(strings.Join(rets, ", "))
			buf.WriteString(")")
		}
	}

	return buf.String()
}

// exprToString converts an AST expression to its string representation.
func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + exprToString(e.Elt)
		}
		return "[" + exprToString(e.Len) + "]" + exprToString(e.Elt)
	case *ast.MapType:
		return "map[" + exprToString(e.Key) + "]" + exprToString(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.ChanType:
		switch e.Dir {
		case ast.SEND:
			return "chan<- " + exprToString(e.Value)
		case ast.RECV:
			return "<-chan " + exprToString(e.Value)
		default:
			return "chan " + exprToString(e.Value)
		}
	case *ast.FuncType:
		return "func(...)"
	case *ast.Ellipsis:
		return "..." + exprToString(e.Elt)
	case *ast.BasicLit:
		return e.Value
	default:
		// Fallback: use go/format
		var buf bytes.Buffer
		if err := format.Node(&buf, token.NewFileSet(), expr); err == nil {
			return buf.String()
		}
		return fmt.Sprintf("%T", expr)
	}
}

// baseTypeName extracts the base type name from an expression (strips * prefix).
func baseTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return baseTypeName(e.X)
	case *ast.SelectorExpr:
		return e.Sel.Name
	default:
		return exprToString(expr)
	}
}

// receiverBaseName extracts the base type name from a receiver string like "*MyStruct".
func receiverBaseName(recv string) string {
	recv = strings.TrimPrefix(recv, "*")
	// Handle qualified names
	if idx := strings.LastIndex(recv, "."); idx >= 0 {
		recv = recv[idx+1:]
	}
	return recv
}

// nodeToString renders an AST node back to source code.
func nodeToString(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return ""
	}
	return buf.String()
}

// computeStats aggregates counts across all packages.
func computeStats(pkgs []*PackageInfo) Stats {
	var s Stats
	s.PackageCount = len(pkgs)
	for _, pkg := range pkgs {
		s.FunctionCount += len(pkg.Functions)
		s.TypeCount += len(pkg.Types)
		s.ConstantCount += len(pkg.Constants)
		s.VariableCount += len(pkg.Variables)
		for _, t := range pkg.Types {
			s.MethodCount += len(t.Methods)
			if t.Kind == TypeKindInterface {
				s.InterfaceCount++
			}
		}
	}
	return s
}
