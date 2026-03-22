package server

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogBufferNewLogBuffer(t *testing.T) {
	buf := NewLogBuffer(100)
	require.NotNil(t, buf)
	assert.Equal(t, 100, buf.max)
}

func TestLogBufferEmptyDump(t *testing.T) {
	buf := NewLogBuffer(10)
	lines := buf.Dump()
	assert.Empty(t, lines)
}

func TestLogBufferWriteSingleLine(t *testing.T) {
	buf := NewLogBuffer(10)
	n, err := buf.Write([]byte("hello\n"))
	assert.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, []string{"hello"}, buf.Dump())
}

func TestLogBufferWriteMultipleLines(t *testing.T) {
	buf := NewLogBuffer(10)
	_, err := buf.Write([]byte("line1\nline2\n"))
	assert.NoError(t, err)
	assert.Equal(t, []string{"line1", "line2"}, buf.Dump())
}

func TestLogBufferWrapAround(t *testing.T) {
	buf := NewLogBuffer(3)
	for i := 0; i < 5; i++ {
		_, err := buf.Write([]byte(fmt.Sprintf("line%d\n", i)))
		require.NoError(t, err)
	}
	// Should have last 3 lines in chronological order
	lines := buf.Dump()
	assert.Equal(t, []string{"line2", "line3", "line4"}, lines)
}

func TestLogBufferSkipsEmptyLines(t *testing.T) {
	buf := NewLogBuffer(10)
	_, err := buf.Write([]byte("a\n\nb\n"))
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, buf.Dump())
}

func TestLogBufferConcurrentSafety(t *testing.T) {
	buf := NewLogBuffer(100)
	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				buf.Write([]byte(fmt.Sprintf("writer%d-line%d\n", id, j)))
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				buf.Dump()
			}
		}()
	}

	wg.Wait()

	lines := buf.Dump()
	assert.LessOrEqual(t, len(lines), 100)
	assert.Greater(t, len(lines), 0)
}
