package basic

// BaseType is an embedded base.
type BaseType struct{}

// MyStruct is a test struct.
type MyStruct struct {
	BaseType
	Name string `json:"name"`
	Age  int
}

// Method1 is a method on MyStruct.
func (m *MyStruct) Method1() string {
	return m.Name
}

// Do implements Doer.
func (m *MyStruct) Do(input string) error {
	return nil
}
