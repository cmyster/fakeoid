package server

import (
	"strings"
	"sync"
)

// LogBuffer is a thread-safe circular buffer for capturing log lines.
type LogBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
	pos   int
	full  bool
}

// NewLogBuffer creates a new LogBuffer that retains up to maxLines.
func NewLogBuffer(maxLines int) *LogBuffer {
	return &LogBuffer{lines: make([]string, maxLines), max: maxLines}
}

// Write implements io.Writer -- splits input into lines and stores them.
func (b *LogBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, line := range strings.Split(string(p), "\n") {
		if line == "" {
			continue
		}
		b.lines[b.pos] = line
		b.pos = (b.pos + 1) % b.max
		if b.pos == 0 {
			b.full = true
		}
	}
	return len(p), nil
}

// Dump returns all stored lines in chronological order.
func (b *LogBuffer) Dump() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.full {
		return append([]string{}, b.lines[:b.pos]...)
	}
	result := make([]string, 0, b.max)
	result = append(result, b.lines[b.pos:]...)
	result = append(result, b.lines[:b.pos]...)
	return result
}
