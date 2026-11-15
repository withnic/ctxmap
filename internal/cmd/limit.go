package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// defaultMaxLines keeps a single command's output within a typical agent's
// context budget. 0 disables the limit.
const defaultMaxLines = 500

func addMaxLinesFlag(fs *flag.FlagSet) *int {
	return fs.Int("max-lines", defaultMaxLines, "stop output after about `N` lines, at a file/directory boundary (0 = unlimited)")
}

func checkMaxLines(n int) error {
	if n < 0 {
		return errors.New("--max-lines must be >= 0")
	}
	return nil
}

// lineBudget caps output at max lines (0 = unlimited). It only ever cuts
// between whole blocks (a file, a directory line), so nothing is printed
// half-way; once one block is dropped, every later block is dropped too so
// the output stays a prefix of the full result.
type lineBudget struct {
	max          int
	used         int
	droppedUnits int
	droppedLines int
}

// newLineBudget starts a budget after the one-line header that every
// limited command prints first.
func newLineBudget(maxLines int) *lineBudget {
	return &lineBudget{max: maxLines, used: 1}
}

// write prints block if it fits in the budget; otherwise it is counted as
// dropped.
func (b *lineBudget) write(w io.Writer, block string) {
	n := strings.Count(block, "\n")
	if b.droppedUnits > 0 || (b.max > 0 && b.used+n > b.max) {
		b.droppedUnits++
		b.droppedLines += n
		return
	}
	b.used += n
	_, _ = io.WriteString(w, block)
}

// truncated prints the trailing notice when anything was dropped. unit is
// the plural noun for a block ("files", "directories"); hint says how to
// narrow the request.
func (b *lineBudget) truncated(w io.Writer, unit, hint string) {
	if b.droppedUnits == 0 {
		return
	}
	fmt.Fprintf(w, "\n... (truncated at --max-lines %d: %d more %s, %d lines not shown; %s, or pass --max-lines 0)\n",
		b.max, b.droppedUnits, unit, b.droppedLines, hint)
}
