package shell

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteToken_WrapsProseAtWidth(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayWriter(&buf, 20)

	// "Hello world this is a test" should wrap
	d.WriteToken("Hello world this is a test")
	d.Flush()

	// "Hello world this is" = 19 chars, fits on line 1
	// "a test" wraps to line 2
	assert.Equal(t, "Hello world this is\na test", buf.String())
}

func TestWriteToken_DoesNotWrapCodeBlocks(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayWriter(&buf, 20)

	d.WriteToken("text\n```\nthis is a very long line of code that should not wrap at all\n```\nmore text")
	d.Flush()

	expected := "text\n```\nthis is a very long line of code that should not wrap at all\n```\nmore text"
	assert.Equal(t, expected, buf.String())
}

func TestWriteToken_PartialWordBuffering(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayWriter(&buf, 10)

	// Token splits in the middle of a word
	d.WriteToken("hel")
	d.WriteToken("lo wor")
	d.WriteToken("ld")
	d.Flush()

	// "hello" = 5, + space + "world" = 5 -> "hello" + space = 6, + "world" = 11 > 10
	// So "world" wraps
	assert.Equal(t, "hello\nworld", buf.String())
}

func TestWriteToken_ResetsColumnOnNewline(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayWriter(&buf, 20)

	d.WriteToken("short\nHello world this is a test")
	d.Flush()

	// After "short\n", col resets to 0
	// Then "Hello world this is" fits (19 chars), "a test" wraps
	assert.Equal(t, "short\nHello world this is\na test", buf.String())
}

func TestWriteToken_BacktickFenceSplitAcrossTokens(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayWriter(&buf, 80)

	d.WriteToken("line\n``")
	d.WriteToken("`go\ncode here\n``")
	d.WriteToken("`\nafter")
	d.Flush()

	expected := "line\n```go\ncode here\n```\nafter"
	assert.Equal(t, expected, buf.String())
}

func TestPrintBanner(t *testing.T) {
	var buf bytes.Buffer
	// Disable color for test output consistency
	ColorBanner.DisableColor()
	defer ColorBanner.EnableColor()

	PrintBanner(&buf, "Qwen2.5-Coder-32B-Q4_K_M", "gfx1100")

	assert.Equal(t, "Model: Qwen2.5-Coder-32B-Q4_K_M | GPU: gfx1100\n", buf.String())
}

func TestWriteToken_ForcBreakLongWord(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayWriter(&buf, 5)

	d.WriteToken("abcdefghij")
	d.Flush()

	// Word is 10 chars, width is 5 -> force-break at 5
	assert.Equal(t, "abcde\nfghij", buf.String())
}

func TestWriteToken_MultipleSpaces(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayWriter(&buf, 40)

	d.WriteToken("hello   world")
	d.Flush()

	// Multiple consecutive spaces collapse to a single deferred space
	// (word-wrap semantics: spaces are word separators, not content)
	assert.Equal(t, "hello world", buf.String())
}

func TestPrintWelcome(t *testing.T) {
	var buf bytes.Buffer
	PrintWelcome(&buf)
	assert.Equal(t, "Describe your task.\n", buf.String())
}

func TestPrintAgentTag(t *testing.T) {
	var buf bytes.Buffer
	ColorAgent.DisableColor()
	defer ColorAgent.EnableColor()

	PrintAgentTag(&buf, 1, "Systems Engineer")
	assert.Equal(t, "[Agent 1: Systems Engineer] ", buf.String())
}

func TestPrintTransition(t *testing.T) {
	var buf bytes.Buffer
	ColorAgent.DisableColor()
	defer ColorAgent.EnableColor()

	PrintTransition(&buf, 4, "Software Engineer")
	assert.Equal(t, "\n--- Handing off to Agent 4: Software Engineer ---\n\n", buf.String())
}

func TestGetTermWidth_FallbackTo80(t *testing.T) {
	// In a test environment (no terminal), should fall back to 80
	width := GetTermWidth()
	assert.True(t, width > 0, "width should be positive")
}

func TestFlush_EmptyWriter(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayWriter(&buf, 80)
	d.Flush()
	assert.Equal(t, "", buf.String())
}

func TestWriteToken_InlineBackticksInProse(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayWriter(&buf, 80)

	d.WriteToken("use `fmt.Println` here")
	d.Flush()

	assert.Equal(t, "use `fmt.Println` here", buf.String())
}

func TestWriteToken_EmptyToken(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplayWriter(&buf, 80)

	d.WriteToken("")
	d.Flush()
	assert.Equal(t, "", buf.String())
}
