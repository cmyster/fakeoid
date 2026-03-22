// Package basic is a test fixture for AST extraction.
package basic

import "fmt"

// DefaultTimeout is the default timeout.
var DefaultTimeout = 30

// StatusActive is an active status.
const StatusActive = "active"

// Status is a named string type.
type Status string

// MyAlias is a type alias.
type MyAlias = string

// Doer defines something that does.
type Doer interface {
	Do(input string) error
}

func init() {
	fmt.Println("init")
}

// DoSomething does something.
func DoSomething(x int) string {
	return fmt.Sprintf("%d", x)
}

func helperFunc() {
	fmt.Println("help")
}

// longFunction has a body longer than 10 lines.
func longFunction() {
	fmt.Println("1")
	fmt.Println("2")
	fmt.Println("3")
	fmt.Println("4")
	fmt.Println("5")
	fmt.Println("6")
	fmt.Println("7")
	fmt.Println("8")
	fmt.Println("9")
	fmt.Println("10")
	fmt.Println("11")
}
