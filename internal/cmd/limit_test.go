package cmd

import (
	"strings"
	"testing"
)

func TestLineBudget(t *testing.T) {
	var b strings.Builder
	budget := newLineBudget(4) // counts the one-line header
	for _, block := range []string{"a\nb\n", "c\n", "d\ne\n", "f\n"} {
		budget.write(&b, block)
	}
	budget.truncated(&b, "files", "hint")
	// "a b" (3 lines) and "c" (4) fit; "d e" would make 6 > 4, so it and every
	// later block are dropped even though "f" alone would still fit.
	want := "a\nb\nc\n\n... (truncated at --max-lines 4: 2 more files, 3 lines not shown; hint, or pass --max-lines 0)\n"
	if b.String() != want {
		t.Errorf("got %q\nwant %q", b.String(), want)
	}

	b.Reset()
	unlimited := newLineBudget(0)
	for range 1000 {
		unlimited.write(&b, "x\n")
	}
	unlimited.truncated(&b, "files", "hint")
	if strings.Contains(b.String(), "truncated") || strings.Count(b.String(), "\n") != 1000 {
		t.Error("--max-lines 0 must not truncate")
	}
	if checkMaxLines(-1) == nil || checkMaxLines(0) != nil {
		t.Error("checkMaxLines: negative values must be rejected, 0 accepted")
	}
}

func TestMaxLinesMap(t *testing.T) {
	files := mustCollect(t, writeTree(t, fixture)).files
	var full strings.Builder
	writeMap(&full, "x", files, mapOpts{}, 0)

	var cut strings.Builder
	writeMap(&cut, "x", files, mapOpts{}, 8)
	out := cut.String()
	if !strings.HasPrefix(out, "# map x (5 files)\n") {
		t.Errorf("header must keep the full file count:\n%s", out)
	}
	if !strings.Contains(out, "more files") || !strings.Contains(out, "ctxmap map --summary") {
		t.Errorf("missing truncation notice:\n%s", out)
	}
	// what is shown is a prefix of the full output, cut at a file boundary
	cutAt := strings.Index(out, "\n... (truncated")
	if cutAt < 0 {
		t.Fatalf("no truncation notice:\n%s", out)
	}
	shown := out[:cutAt]
	if !strings.HasPrefix(full.String(), shown) || !strings.HasPrefix(full.String()[len(shown):], "\n## ") {
		t.Errorf("output must be cut between files:\n%s", out)
	}
	if lines := strings.Count(shown, "\n"); lines > 8 {
		t.Errorf("shown %d lines, budget was 8", lines)
	}
	if strings.Contains(full.String(), "truncated") {
		t.Error("unlimited output must not be truncated")
	}
}

func TestMaxLinesSummaryAndDeps(t *testing.T) {
	s := mustCollect(t, writeTree(t, fixture))

	var b strings.Builder
	writeSummary(&b, "x", s.files, mapOpts{}, 3)
	if out := b.String(); !strings.Contains(out, "# map x --summary (5 files") ||
		strings.Count(out, " files, ") != 3 || // header + 2 directories
		!strings.Contains(out, "3 more directories") {
		t.Errorf("summary truncation:\n%s", out)
	}

	b.Reset()
	g := newDepGraph(s)
	g.writeAll(&b, g.files, false, 2)
	if out := b.String(); !strings.Contains(out, "(local only; 3/5 files have local deps)") ||
		strings.Count(out, " -> ") != 1 ||
		!strings.Contains(out, "2 more files") || !strings.Contains(out, "--target") {
		t.Errorf("deps truncation:\n%s", out)
	}
}
