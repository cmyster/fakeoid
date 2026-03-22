// Package pkga defines interfaces for testing cross-package resolution.
package pkga

// Worker can perform work.
type Worker interface {
	Work() error
}

// Namer can return a name.
type Namer interface {
	Name() string
}
