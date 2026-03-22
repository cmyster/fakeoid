package extract

// TypeKind distinguishes the kind of a type declaration.
type TypeKind string

const (
	TypeKindStruct    TypeKind = "struct"
	TypeKindInterface TypeKind = "interface"
	TypeKindAlias     TypeKind = "alias"
	TypeKindNamed     TypeKind = "named"
)

// Extractor abstracts language-specific codebase extraction.
type Extractor interface {
	Extract(root string) (*ProjectInfo, error)
}

// ProjectInfo holds the complete extraction result for a project.
type ProjectInfo struct {
	ModulePath  string
	Packages    []*PackageInfo
	ImportGraph map[string][]string
	Implements  []InterfaceSatisfaction
	Stats       Stats
	Errors      []string
}

// Stats summarizes extraction counts.
type Stats struct {
	PackageCount   int
	TypeCount      int
	FunctionCount  int
	MethodCount    int
	InterfaceCount int
	ConstantCount  int
	VariableCount  int
}

// PackageInfo holds extracted data for one package.
type PackageInfo struct {
	Name      string
	ImportPath string
	RelPath   string
	Doc       string
	Imports   []string
	Files     []string
	Functions []FunctionInfo
	Types     []TypeInfo
	Constants []ConstInfo
	Variables []VarInfo
	InitFuncs []FunctionInfo
	BuildTags []string
}

// TypeInfo describes an extracted type declaration.
type TypeInfo struct {
	Name       string
	Kind       TypeKind
	Doc        string
	Fields     []FieldInfo
	Methods    []FunctionInfo
	Implements []string
	Source     string // file:line
}

// FunctionInfo describes an extracted function or method.
type FunctionInfo struct {
	Name      string
	Doc       string
	Signature string
	Receiver  string // empty for package-level functions
	Body      string // included if body <= 10 source lines
	InitFunc  bool   // true for init() functions
	Source    string // file:line
}

// FieldInfo describes a struct field.
type FieldInfo struct {
	Name     string
	Type     string
	Tag      string
	Doc      string
	Embedded bool
}

// ConstInfo describes a constant.
type ConstInfo struct {
	Name   string
	Type   string
	Value  string
	Doc    string
	Source string
}

// VarInfo describes a package-level variable.
type VarInfo struct {
	Name   string
	Type   string
	Doc    string
	Source string
}

// InterfaceSatisfaction records that a concrete type implements an interface.
type InterfaceSatisfaction struct {
	Type      string
	Interface string
}
