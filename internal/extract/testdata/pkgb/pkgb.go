// Package pkgb implements pkga interfaces.
package pkgb

import "testmod/pkga"

// Ensure interface compliance.
var _ pkga.Worker = (*MyWorker)(nil)

// MyWorker implements Worker.
type MyWorker struct {
	label string
}

// Work performs the work.
func (w *MyWorker) Work() error {
	return nil
}

// Name returns the worker name.
func (w *MyWorker) Name() string {
	return w.label
}
