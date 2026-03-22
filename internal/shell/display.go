package shell

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/cmyster/fakeoid/internal/state"
	"github.com/fatih/color"
	"golang.org/x/term"
)

// Color palette: 4 colors for the entire UI.
var (
	ColorPrompt = color.New(color.FgGreen)  // fakeoid> prompt
	ColorAgent  = color.New(color.FgCyan)   // [Agent N] tag
	ColorError  = color.New(color.FgRed)    // error messages
	ColorBanner = color.New(color.FgYellow) // startup banner
)

// DisplayWriter handles streaming token output with word-wrap at terminal width.
// Prose text wraps at word boundaries; fenced code blocks (```) pass through unwrapped.
type DisplayWriter struct {
	width    int
	col      int
	inCode   bool
	out      io.Writer
	wordBuf  strings.Builder // accumulates current word (non-whitespace run)
	tickBuf  int             // accumulated backtick characters for ``` detection
	lineStart bool           // true when at the start of a line (for detecting ``` fences)
	needSpace bool           // deferred space: emit only if next word stays on same line
}

// NewDisplayWriter creates a DisplayWriter that wraps prose at the given width.
func NewDisplayWriter(out io.Writer, width int) *DisplayWriter {
	if width <= 0 {
		width = 80
	}
	return &DisplayWriter{
		width:     width,
		out:       out,
		lineStart: true,
	}
}

// WriteToken processes a streaming token fragment, handling word-wrap and code block detection.
func (d *DisplayWriter) WriteToken(token string) {
	for _, ch := range token {
		if ch == '`' {
			d.tickBuf++
			if d.tickBuf == 3 && d.lineStart {
				// Toggle code block mode
				d.flushWord()
				d.needSpace = false
				d.inCode = !d.inCode
				d.tickBuf = 0
				fmt.Fprint(d.out, "```")
				continue
			}
			continue
		}

		// If we had accumulated backticks but didn't reach 3, flush them
		if d.tickBuf > 0 {
			d.emitTicks()
		}

		if ch == '\n' {
			d.flushWord()
			d.needSpace = false
			fmt.Fprint(d.out, "\n")
			d.col = 0
			d.lineStart = true
			continue
		}

		d.lineStart = false

		if d.inCode {
			// Inside code block: write as-is, no wrapping
			fmt.Fprintf(d.out, "%c", ch)
			d.col++
			continue
		}

		// Prose mode: word-wrap at width
		if ch == ' ' || ch == '\t' {
			d.flushWord()
			// Defer the space -- only emit it when the next word is written on the same line.
			// This avoids trailing spaces at line boundaries.
			if d.col > 0 {
				d.needSpace = true
			}
		} else {
			d.wordBuf.WriteRune(ch)
		}
	}
}

// emitTicks writes accumulated backtick characters that didn't form a ``` fence.
func (d *DisplayWriter) emitTicks() {
	ticks := strings.Repeat("`", d.tickBuf)
	d.tickBuf = 0

	if d.inCode {
		fmt.Fprint(d.out, ticks)
		d.col += len(ticks)
		return
	}

	// In prose mode, treat backticks as part of word buffer
	d.wordBuf.WriteString(ticks)
}

// flushWord writes the buffered word to output, wrapping to a new line if needed.
func (d *DisplayWriter) flushWord() {
	if d.wordBuf.Len() == 0 {
		return
	}

	word := d.wordBuf.String()
	d.wordBuf.Reset()

	wordLen := len(word)

	// If word is longer than entire width, force-break it
	if wordLen > d.width {
		d.needSpace = false
		for _, ch := range word {
			if d.col >= d.width {
				fmt.Fprint(d.out, "\n")
				d.col = 0
			}
			fmt.Fprintf(d.out, "%c", ch)
			d.col++
		}
		return
	}

	// Calculate space needed: word + 1 for the deferred space (if any)
	spaceNeeded := wordLen
	if d.needSpace {
		spaceNeeded++ // account for the space we'll emit before the word
	}

	// If word (plus space) doesn't fit on current line, wrap
	if d.col+spaceNeeded > d.width && d.col > 0 {
		fmt.Fprint(d.out, "\n")
		d.col = 0
		d.needSpace = false // space is consumed by the line break
	}

	// Emit deferred space if still needed
	if d.needSpace {
		fmt.Fprint(d.out, " ")
		d.col++
		d.needSpace = false
	}

	fmt.Fprint(d.out, word)
	d.col += wordLen
}

// Flush writes any remaining buffered content to the output.
func (d *DisplayWriter) Flush() {
	if d.tickBuf > 0 {
		d.emitTicks()
	}
	d.flushWord()
}

// PrintBanner writes the startup banner: "Model: {name} | GPU: {gpu}".
func PrintBanner(out io.Writer, modelName string, gpuName string) {
	ColorBanner.Fprintf(out, "Model: %s | GPU: %s\n", modelName, gpuName)
}

// PrintWelcome writes the welcome message.
func PrintWelcome(out io.Writer) {
	fmt.Fprintln(out, "Describe your task.")
}

// PrintAgentTag writes the agent identity tag: "[Agent N: Name] ".
func PrintAgentTag(out io.Writer, agentNum int, agentName string) {
	ColorAgent.Fprintf(out, "[Agent %d: %s] ", agentNum, agentName)
}

// PrintTransition writes a colored agent handoff banner.
func PrintTransition(out io.Writer, agentNum int, agentName string) {
	ColorAgent.Fprintf(out, "\n--- Handing off to Agent %d: %s ---\n\n", agentNum, agentName)
}

// PrintHistoryTable prints a compact table of recent task history records.
// taskNameTruncLen controls how long task names can be before truncation (0 = 40).
func PrintHistoryTable(out io.Writer, records []state.HistoryRecord, taskNameTruncLen int) {
	if taskNameTruncLen == 0 {
		taskNameTruncLen = 40
	}
	fmt.Fprintln(out, "Recent tasks:")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tDate\tTask\tResult")
	for i, r := range records {
		taskName := r.TaskName
		if len(taskName) > taskNameTruncLen {
			taskName = taskName[:taskNameTruncLen]
		}
		var symbol string
		switch r.Outcome {
		case "success":
			symbol = "\u2713"
		case "failure", "interrupted":
			symbol = "X"
		default:
			symbol = "?"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", i+1, r.Timestamp.Format("2006-01-02"), taskName, symbol)
	}
	w.Flush()
}

// PrintFirstRun prints a message for first-time use in a project.
func PrintFirstRun(out io.Writer) {
	fmt.Fprintln(out, "First session in this project. Task history will be saved to .fakeoid/")
}

// PrintPipelineSummary prints a brief status line showing how many agents ran and which were skipped.
func PrintPipelineSummary(out io.Writer, outcomes []state.AgentOutcome) {
	ran := 0
	var skipped []string
	for _, o := range outcomes {
		if o.Status != "skipped" {
			ran++
		} else {
			skipped = append(skipped, fmt.Sprintf("Agent %d", o.Number))
		}
	}
	ColorAgent.Fprintf(out, "\nPipeline complete: %d/%d agents ran", ran, len(outcomes))
	if len(skipped) > 0 {
		ColorAgent.Fprintf(out, " (skipped: %s)", strings.Join(skipped, ", "))
	}
	fmt.Fprintln(out)
}

// PrintHistoryDetail prints a detailed view of a single task's execution,
// showing the overall outcome and per-agent status breakdown.
func PrintHistoryDetail(out io.Writer, taskName string, fm state.TaskFrontmatter) {
	fmt.Fprintf(out, "\nTask: %s\n", taskName)
	fmt.Fprintf(out, "Date: %s\n", fm.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(out, "Outcome: %s\n", fm.Outcome)
	if fm.DurationSec > 0 {
		fmt.Fprintf(out, "Duration: %.1fs\n", fm.DurationSec)
	}
	if fm.TestResult != "" {
		fmt.Fprintf(out, "Test result: %s\n", fm.TestResult)
	}
	fmt.Fprintln(out)

	if len(fm.Agents) == 0 {
		fmt.Fprintln(out, "No per-agent data available (pre-Phase 13 task)")
		return
	}

	fmt.Fprintln(out, "Agent outcomes:")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Agent\tName\tStatus")
	for _, a := range fm.Agents {
		var symbol string
		switch a.Status {
		case "success":
			symbol = "\u2713 success"
		case "failed":
			symbol = "X failed"
		case "skipped":
			symbol = "- skipped"
		default:
			symbol = "? " + a.Status
		}
		fmt.Fprintf(w, "%d\t%s\t%s\n", a.Number, a.Name, symbol)
	}
	w.Flush()
}

// GetTermWidth returns the current terminal width, falling back to 80 if detection fails.
func GetTermWidth() int {
	return GetTermWidthWithFallback(80)
}

// GetTermWidthWithFallback returns the current terminal width, falling back to
// the given value if detection fails.
func GetTermWidthWithFallback(fallback int) int {
	if fallback <= 0 {
		fallback = 80
	}
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return fallback
	}
	return width
}
