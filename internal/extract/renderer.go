package extract

import (
	"fmt"
	"sort"
	"strings"
)

// RenderMarkdown produces a structured markdown representation of the
// extracted project info, suitable for injection into an LLM system prompt.
func RenderMarkdown(info *ProjectInfo) string {
	var b strings.Builder

	b.WriteString("# Codebase Structure\n\n")

	// Stats section
	renderStats(&b, info.Stats)

	// Interface implementations
	renderImplementations(&b, info.Implements)

	// Import graph
	renderImportGraph(&b, info.ImportGraph)

	// Per-package sections (sorted alphabetically)
	pkgs := make([]*PackageInfo, len(info.Packages))
	copy(pkgs, info.Packages)
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].ImportPath < pkgs[j].ImportPath
	})

	for _, pkg := range pkgs {
		renderPackage(&b, pkg)
	}

	// Errors section
	renderErrors(&b, info.Errors)

	return b.String()
}

func renderStats(b *strings.Builder, s Stats) {
	b.WriteString("## Stats\n\n")
	fmt.Fprintf(b, "- Packages: %d\n", s.PackageCount)
	fmt.Fprintf(b, "- Types: %d (%d interfaces)\n", s.TypeCount, s.InterfaceCount)
	fmt.Fprintf(b, "- Functions: %d\n", s.FunctionCount)
	fmt.Fprintf(b, "- Methods: %d\n", s.MethodCount)
	fmt.Fprintf(b, "- Constants: %d\n", s.ConstantCount)
	fmt.Fprintf(b, "- Variables: %d\n", s.VariableCount)
	b.WriteString("\n")
}

func renderImplementations(b *strings.Builder, impls []InterfaceSatisfaction) {
	if len(impls) == 0 {
		return
	}

	b.WriteString("## Interface Implementations\n\n")

	// Sort for stable output
	sorted := make([]InterfaceSatisfaction, len(impls))
	copy(sorted, impls)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Type == sorted[j].Type {
			return sorted[i].Interface < sorted[j].Interface
		}
		return sorted[i].Type < sorted[j].Type
	})

	for _, impl := range sorted {
		fmt.Fprintf(b, "- `%s` implements `%s`\n", shortName(impl.Type), shortName(impl.Interface))
	}
	b.WriteString("\n")
}

func renderImportGraph(b *strings.Builder, graph map[string][]string) {
	if len(graph) == 0 {
		return
	}

	b.WriteString("## Import Graph\n\n")

	// Sort keys for stable output
	keys := make([]string, 0, len(graph))
	for k := range graph {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, pkg := range keys {
		imports := graph[pkg]
		sort.Strings(imports)
		fmt.Fprintf(b, "- `%s` imports: %s\n", shortName(pkg), formatImportList(imports))
	}
	b.WriteString("\n")
}

func renderPackage(b *strings.Builder, pkg *PackageInfo) {
	// Heading
	if pkg.ImportPath != "" {
		fmt.Fprintf(b, "## Package: %s (`%s`)\n\n", pkg.Name, pkg.ImportPath)
	} else {
		fmt.Fprintf(b, "## Package: %s\n\n", pkg.Name)
	}

	if pkg.Doc != "" {
		b.WriteString(pkg.Doc)
		b.WriteString("\n\n")
	}

	if len(pkg.Files) > 0 {
		sort.Strings(pkg.Files)
		fmt.Fprintf(b, "Files: %s\n\n", strings.Join(pkg.Files, ", "))
	}

	// Types (sorted alphabetically)
	if len(pkg.Types) > 0 {
		types := make([]TypeInfo, len(pkg.Types))
		copy(types, pkg.Types)
		sort.Slice(types, func(i, j int) bool {
			return types[i].Name < types[j].Name
		})

		b.WriteString("### Types\n\n")
		for _, ty := range types {
			renderType(b, &ty)
		}
	}

	// Functions (sorted alphabetically)
	if len(pkg.Functions) > 0 {
		fns := make([]FunctionInfo, len(pkg.Functions))
		copy(fns, pkg.Functions)
		sort.Slice(fns, func(i, j int) bool {
			return fns[i].Name < fns[j].Name
		})

		b.WriteString("### Functions\n\n")
		for _, fn := range fns {
			renderFunction(b, &fn)
		}
	}

	// Constants
	if len(pkg.Constants) > 0 {
		b.WriteString("### Constants\n\n")
		for _, c := range pkg.Constants {
			if c.Value != "" {
				fmt.Fprintf(b, "- `%s` = `%s`", c.Name, c.Value)
			} else {
				fmt.Fprintf(b, "- `%s`", c.Name)
			}
			if c.Type != "" {
				fmt.Fprintf(b, " (%s)", c.Type)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Variables
	if len(pkg.Variables) > 0 {
		b.WriteString("### Variables\n\n")
		for _, v := range pkg.Variables {
			fmt.Fprintf(b, "- `%s`", v.Name)
			if v.Type != "" {
				fmt.Fprintf(b, " (%s)", v.Type)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Init functions
	if len(pkg.InitFuncs) > 0 {
		b.WriteString("### Init Functions\n\n")
		for _, fn := range pkg.InitFuncs {
			fmt.Fprintf(b, "- init() in %s\n", fn.Source)
		}
		b.WriteString("\n")
	}
}

func renderType(b *strings.Builder, ty *TypeInfo) {
	fmt.Fprintf(b, "#### type %s %s\n\n", ty.Name, ty.Kind)

	if ty.Doc != "" {
		b.WriteString(ty.Doc)
		b.WriteString("\n\n")
	}

	// Struct fields as code block
	if ty.Kind == TypeKindStruct && len(ty.Fields) > 0 {
		b.WriteString("```go\n")
		fmt.Fprintf(b, "type %s struct {\n", ty.Name)
		for _, f := range ty.Fields {
			if f.Embedded {
				fmt.Fprintf(b, "\t%s\n", f.Type)
			} else if f.Tag != "" {
				fmt.Fprintf(b, "\t%s %s `%s`\n", f.Name, f.Type, f.Tag)
			} else {
				fmt.Fprintf(b, "\t%s %s\n", f.Name, f.Type)
			}
		}
		b.WriteString("}\n```\n\n")
	}

	// Methods
	if len(ty.Methods) > 0 {
		methods := make([]FunctionInfo, len(ty.Methods))
		copy(methods, ty.Methods)
		sort.Slice(methods, func(i, j int) bool {
			return methods[i].Name < methods[j].Name
		})

		b.WriteString("**Methods:**\n\n")
		for _, m := range methods {
			fmt.Fprintf(b, "```go\n%s\n```\n\n", m.Signature)
		}
	}

	// Implements
	if len(ty.Implements) > 0 {
		b.WriteString("**Implements:** ")
		names := make([]string, len(ty.Implements))
		for i, iface := range ty.Implements {
			names[i] = shortName(iface)
		}
		b.WriteString(strings.Join(names, ", "))
		b.WriteString("\n\n")
	}
}

func renderFunction(b *strings.Builder, fn *FunctionInfo) {
	if fn.Doc != "" {
		fmt.Fprintf(b, "```go\n// %s\n%s\n```\n\n", fn.Doc, fn.Signature)
	} else {
		fmt.Fprintf(b, "```go\n%s\n```\n\n", fn.Signature)
	}

	if fn.Body != "" {
		b.WriteString("<details><summary>Body</summary>\n\n")
		fmt.Fprintf(b, "```go\n%s\n```\n\n", fn.Body)
		b.WriteString("</details>\n\n")
	}
}

func renderErrors(b *strings.Builder, errors []string) {
	if len(errors) == 0 {
		return
	}

	b.WriteString("## Errors\n\n")
	for _, e := range errors {
		fmt.Fprintf(b, "- %s\n", e)
	}
	b.WriteString("\n")
}

// shortName trims a fully qualified name to its last two segments
// (e.g., "example.com/myproject/mypkg.Widget" -> "mypkg.Widget").
func shortName(fqn string) string {
	// Find the last slash
	lastSlash := strings.LastIndex(fqn, "/")
	if lastSlash >= 0 {
		return fqn[lastSlash+1:]
	}
	return fqn
}

// formatImportList formats a list of import paths for display.
func formatImportList(imports []string) string {
	short := make([]string, len(imports))
	for i, imp := range imports {
		short[i] = "`" + shortName(imp) + "`"
	}
	return strings.Join(short, ", ")
}
