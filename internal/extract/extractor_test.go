package extract

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func extractBasic(t *testing.T) *ProjectInfo {
	t.Helper()
	ext := NewGoExtractor()
	info, err := ext.Extract("testdata/basic")
	require.NoError(t, err)
	require.NotNil(t, info)
	require.NotEmpty(t, info.Packages)
	return info
}

func TestExtractFunctions(t *testing.T) {
	info := extractBasic(t)
	pkg := info.Packages[0]

	funcNames := make(map[string]bool)
	for _, f := range pkg.Functions {
		funcNames[f.Name] = true
	}

	assert.True(t, funcNames["DoSomething"], "should find exported func DoSomething")
	assert.True(t, funcNames["helperFunc"], "should find unexported func helperFunc")

	// Verify DoSomething signature contains parameter and return type
	for _, f := range pkg.Functions {
		if f.Name == "DoSomething" {
			assert.Contains(t, f.Signature, "int")
			assert.Contains(t, f.Signature, "string")
		}
	}
}

func TestExtractTypes(t *testing.T) {
	info := extractBasic(t)
	pkg := info.Packages[0]

	typeMap := make(map[string]TypeInfo)
	for _, ty := range pkg.Types {
		typeMap[ty.Name] = ty
	}

	assert.Contains(t, typeMap, "MyStruct")
	assert.Equal(t, TypeKindStruct, typeMap["MyStruct"].Kind)

	assert.Contains(t, typeMap, "Doer")
	assert.Equal(t, TypeKindInterface, typeMap["Doer"].Kind)

	assert.Contains(t, typeMap, "MyAlias")
	assert.Equal(t, TypeKindAlias, typeMap["MyAlias"].Kind)

	assert.Contains(t, typeMap, "Status")
	assert.Equal(t, TypeKindNamed, typeMap["Status"].Kind)
}

func TestStructFields(t *testing.T) {
	info := extractBasic(t)
	pkg := info.Packages[0]

	var myStruct *TypeInfo
	for i, ty := range pkg.Types {
		if ty.Name == "MyStruct" {
			myStruct = &pkg.Types[i]
			break
		}
	}
	require.NotNil(t, myStruct, "MyStruct type must exist")

	fieldMap := make(map[string]FieldInfo)
	for _, f := range myStruct.Fields {
		fieldMap[f.Name] = f
	}

	// Name field with json tag
	require.Contains(t, fieldMap, "Name")
	assert.Equal(t, "string", fieldMap["Name"].Type)
	assert.Contains(t, fieldMap["Name"].Tag, `json:"name"`)

	// Age field without tag
	require.Contains(t, fieldMap, "Age")
	assert.Equal(t, "int", fieldMap["Age"].Type)
	assert.Empty(t, fieldMap["Age"].Tag)

	// BaseType embedded field
	require.Contains(t, fieldMap, "BaseType")
	assert.True(t, fieldMap["BaseType"].Embedded)
}

func TestDocComments(t *testing.T) {
	info := extractBasic(t)
	pkg := info.Packages[0]

	for _, f := range pkg.Functions {
		if f.Name == "DoSomething" {
			assert.Contains(t, f.Doc, "DoSomething does something.")
			return
		}
	}
	t.Fatal("DoSomething not found in functions")
}

func TestMethodGrouping(t *testing.T) {
	info := extractBasic(t)
	pkg := info.Packages[0]

	var myStruct *TypeInfo
	for i, ty := range pkg.Types {
		if ty.Name == "MyStruct" {
			myStruct = &pkg.Types[i]
			break
		}
	}
	require.NotNil(t, myStruct)

	methodNames := make(map[string]bool)
	for _, m := range myStruct.Methods {
		methodNames[m.Name] = true
	}

	assert.True(t, methodNames["Method1"], "Method1 should be grouped under MyStruct")
	assert.True(t, methodNames["Do"], "Do should be grouped under MyStruct")

	// Methods should NOT appear in the flat functions list
	for _, f := range pkg.Functions {
		assert.NotEqual(t, "Method1", f.Name, "Method1 should not be in flat functions list")
		assert.NotEqual(t, "Do", f.Name, "Do should not be in flat functions list")
	}
}

func TestExclusions(t *testing.T) {
	ext := NewGoExtractor()

	// Generated files should be excluded
	info, err := ext.Extract("testdata/generated")
	require.NoError(t, err)
	require.NotNil(t, info)

	if len(info.Packages) > 0 {
		pkg := info.Packages[0]
		assert.Empty(t, pkg.Functions, "generated file functions should be excluded")
		assert.Empty(t, pkg.Types, "generated file types should be excluded")
	}
}

func TestInitFunction(t *testing.T) {
	info := extractBasic(t)
	pkg := info.Packages[0]

	require.NotEmpty(t, pkg.InitFuncs, "init functions should be extracted")
	assert.Equal(t, "init", pkg.InitFuncs[0].Name)
	assert.True(t, pkg.InitFuncs[0].InitFunc, "init function should be flagged")
}

func TestConstants(t *testing.T) {
	info := extractBasic(t)
	pkg := info.Packages[0]

	constMap := make(map[string]ConstInfo)
	for _, c := range pkg.Constants {
		constMap[c.Name] = c
	}

	require.Contains(t, constMap, "StatusActive")
	assert.Equal(t, `"active"`, constMap["StatusActive"].Value)
}

func TestPackageVars(t *testing.T) {
	info := extractBasic(t)
	pkg := info.Packages[0]

	varMap := make(map[string]VarInfo)
	for _, v := range pkg.Variables {
		varMap[v.Name] = v
	}

	require.Contains(t, varMap, "DefaultTimeout")
}

func TestShortBodyIncluded(t *testing.T) {
	info := extractBasic(t)
	pkg := info.Packages[0]

	for _, f := range pkg.Functions {
		if f.Name == "DoSomething" {
			assert.NotEmpty(t, f.Body, "short function body should be included")
			return
		}
	}
	t.Fatal("DoSomething not found")
}

func TestLongBodyOmitted(t *testing.T) {
	info := extractBasic(t)
	pkg := info.Packages[0]

	for _, f := range pkg.Functions {
		if f.Name == "longFunction" {
			assert.Empty(t, f.Body, "long function body should be omitted")
			return
		}
	}
	t.Fatal("longFunction not found")
}

func extractCrossPkg(t *testing.T) *ProjectInfo {
	t.Helper()
	ext := NewGoExtractor()
	info, err := ext.Extract("testdata")
	require.NoError(t, err)
	require.NotNil(t, info)
	return info
}

func TestCrossPackageResolution(t *testing.T) {
	info := extractCrossPkg(t)

	// Should find that MyWorker implements Worker across packages
	found := false
	for _, impl := range info.Implements {
		if impl.Type == "testmod/pkgb.MyWorker" && impl.Interface == "testmod/pkga.Worker" {
			found = true
			break
		}
	}
	assert.True(t, found, "should detect MyWorker implements Worker across packages, got: %v", info.Implements)
}

func TestImportGraph(t *testing.T) {
	info := extractCrossPkg(t)

	// pkgb imports pkga
	imports, ok := info.ImportGraph["testmod/pkgb"]
	require.True(t, ok, "import graph should have testmod/pkgb entry")
	assert.Contains(t, imports, "testmod/pkga", "pkgb should import pkga")
}

func TestInterfaceSatisfactionOnType(t *testing.T) {
	info := extractCrossPkg(t)

	// Find pkgb's MyWorker type and check its Implements field
	for _, pkg := range info.Packages {
		if pkg.Name == "pkgb" {
			for _, ty := range pkg.Types {
				if ty.Name == "MyWorker" {
					assert.Contains(t, ty.Implements, "testmod/pkga.Worker",
						"MyWorker TypeInfo.Implements should list Worker")
					return
				}
			}
		}
	}
	t.Fatal("MyWorker not found in pkgb")
}

func TestPointerReceiverImplements(t *testing.T) {
	info := extractCrossPkg(t)

	// MyWorker uses pointer receiver (*MyWorker), should still detect satisfaction
	found := false
	for _, impl := range info.Implements {
		if impl.Type == "testmod/pkgb.MyWorker" && impl.Interface == "testmod/pkga.Worker" {
			found = true
			break
		}
	}
	assert.True(t, found, "pointer receiver methods should still satisfy interface")
}

func TestErrorReporting(t *testing.T) {
	// Create a temporary directory with a syntax error file
	tmpDir := t.TempDir()

	// Write go.mod
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module errmod\n\ngo 1.25\n"), 0644))

	// Write a broken Go file
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "broken"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "broken", "bad.go"),
		[]byte("package broken\n\nfunc Bad( {\n}\n"), 0644))

	ext := NewGoExtractor()
	info, err := ext.Extract(tmpDir)
	require.NoError(t, err, "Extract should not return error for parse errors")
	require.NotNil(t, info)

	// Errors should be reported in info.Errors, not as panics
	assert.NotEmpty(t, info.Errors, "parse errors should be reported in Errors slice")
}

func TestSelfExtract(t *testing.T) {
	// Locate fakeoid root: this file is at fakeoid/internal/extract/extractor_test.go
	// Go up 2 levels from extract/ -> internal/ -> fakeoid/
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fakeoidRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	ext := NewGoExtractor()
	info, err := ext.Extract(fakeoidRoot)
	require.NoError(t, err)
	require.NotNil(t, info)

	// Should have a meaningful number of packages
	assert.GreaterOrEqual(t, info.Stats.PackageCount, 8,
		"fakeoid should have at least 8 packages")
	assert.Greater(t, info.Stats.FunctionCount, 0,
		"fakeoid should have functions")

	// Should find at least one interface satisfaction
	assert.NotEmpty(t, info.Implements,
		"fakeoid should have at least one interface satisfaction entry")

	// Should find the Agent interface
	foundAgent := false
	for _, pkg := range info.Packages {
		for _, ty := range pkg.Types {
			if ty.Name == "Agent" && ty.Kind == TypeKindInterface {
				foundAgent = true
				break
			}
		}
	}
	assert.True(t, foundAgent, "should find Agent interface from internal/agent")
}
