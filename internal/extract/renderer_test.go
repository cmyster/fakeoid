package extract

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleProjectInfo() *ProjectInfo {
	return &ProjectInfo{
		ModulePath: "example.com/myproject",
		Packages: []*PackageInfo{
			{
				Name:       "mypkg",
				ImportPath: "example.com/myproject/mypkg",
				RelPath:    "mypkg",
				Doc:        "Package mypkg does things.",
				Files:      []string{"mypkg.go", "helpers.go"},
				Imports:    []string{"fmt", "strings"},
				Functions: []FunctionInfo{
					{
						Name:      "NewWidget",
						Doc:       "NewWidget creates a widget.",
						Signature: "func NewWidget(name string) *Widget",
						Source:    "mypkg.go:10",
						Body:      `{ return &Widget{Name: name} }`,
					},
				},
				Types: []TypeInfo{
					{
						Name: "Widget",
						Kind: TypeKindStruct,
						Doc:  "Widget represents a widget.",
						Fields: []FieldInfo{
							{Name: "Name", Type: "string", Tag: `json:"name"`},
							{Name: "Count", Type: "int"},
						},
						Methods: []FunctionInfo{
							{
								Name:      "Run",
								Signature: "func (w *Widget) Run() error",
								Receiver:  "*Widget",
								Source:    "mypkg.go:20",
							},
						},
						Implements: []string{"example.com/myproject/iface.Runner"},
						Source:     "mypkg.go:5",
					},
					{
						Name: "Runner",
						Kind: TypeKindInterface,
						Doc:  "Runner runs things.",
					},
				},
				Constants: []ConstInfo{
					{Name: "StatusActive", Type: "Status", Value: `"active"`, Source: "mypkg.go:30"},
				},
				Variables: []VarInfo{
					{Name: "DefaultTimeout", Type: "int", Source: "mypkg.go:35"},
				},
				InitFuncs: []FunctionInfo{
					{Name: "init", InitFunc: true, Source: "mypkg.go:40"},
				},
			},
		},
		ImportGraph: map[string][]string{
			"example.com/myproject/mypkg": {"fmt", "strings"},
		},
		Implements: []InterfaceSatisfaction{
			{Type: "example.com/myproject/mypkg.Widget", Interface: "example.com/myproject/iface.Runner"},
		},
		Stats: Stats{
			PackageCount:   1,
			TypeCount:      2,
			FunctionCount:  1,
			MethodCount:    1,
			InterfaceCount: 1,
			ConstantCount:  1,
			VariableCount:  1,
		},
		Errors: []string{"// ERROR: could not resolve type X"},
	}
}

func TestRenderStats(t *testing.T) {
	info := sampleProjectInfo()
	out := RenderMarkdown(info)

	assert.Contains(t, out, "## Stats")
	assert.Contains(t, out, "Packages: 1")
	assert.Contains(t, out, "Types: 2")
	assert.Contains(t, out, "Functions: 1")
}

func TestRenderPackageHeading(t *testing.T) {
	info := sampleProjectInfo()
	out := RenderMarkdown(info)

	assert.Contains(t, out, "## Package: mypkg")
	assert.Contains(t, out, "example.com/myproject/mypkg")
}

func TestRenderFunctionSignature(t *testing.T) {
	info := sampleProjectInfo()
	out := RenderMarkdown(info)

	assert.Contains(t, out, "func NewWidget(name string) *Widget")
}

func TestRenderStructFields(t *testing.T) {
	info := sampleProjectInfo()
	out := RenderMarkdown(info)

	assert.Contains(t, out, "Name")
	assert.Contains(t, out, "string")
	assert.Contains(t, out, `json:"name"`)
}

func TestRenderMethodsGrouped(t *testing.T) {
	info := sampleProjectInfo()
	out := RenderMarkdown(info)

	// Methods should appear under the Widget type section
	widgetIdx := strings.Index(out, "Widget")
	runIdx := strings.Index(out, "func (w *Widget) Run()")
	assert.Greater(t, runIdx, widgetIdx, "Run method should appear after Widget type heading")
}

func TestRenderInterfaceSatisfaction(t *testing.T) {
	info := sampleProjectInfo()
	out := RenderMarkdown(info)

	assert.Contains(t, out, "## Interface Implementations")
	assert.Contains(t, out, "mypkg.Widget")
	assert.Contains(t, out, "iface.Runner")
}

func TestRenderImportGraph(t *testing.T) {
	info := sampleProjectInfo()
	out := RenderMarkdown(info)

	assert.Contains(t, out, "## Import Graph")
	assert.Contains(t, out, "mypkg")
}

func TestRenderErrors(t *testing.T) {
	info := sampleProjectInfo()
	out := RenderMarkdown(info)

	assert.Contains(t, out, "## Errors")
	assert.Contains(t, out, "// ERROR: could not resolve type X")
}

func TestRenderFullOutput(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fakeoidRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	ext := NewGoExtractor()
	info, err := ext.Extract(fakeoidRoot)
	require.NoError(t, err)

	out := RenderMarkdown(info)
	assert.NotEmpty(t, out)
	assert.Contains(t, out, "# Codebase Structure")
	assert.Contains(t, out, "## Stats")
	assert.Contains(t, out, "## Package:")
}
