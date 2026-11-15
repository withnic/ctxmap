package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/withnic/ctxmap/internal/parser"
)

// hit is a symbol candidate with its qualified name (Name or Recv.Method).
type hit struct {
	f    *source
	sym  parser.Symbol
	name string
}

// Slice implements `ctxmap slice <symbol>... [path]`: print the full
// definition (body included) of functions, methods, types, consts or vars.
func Slice(args []string) int {
	fs := newFlagSet("slice", "slice <symbol>[,<symbol>...] [<symbol>...] [path] [flags]\n  symbol: Name | Recv.Method")
	sf := addScanFlags(fs)
	pathFlag := fs.String("path", "", "file or directory to search (default \".\"); use it when a symbol could be mistaken for a path")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return flagExit(err)
	}
	queries, root := splitSliceArgs(pos, *pathFlag)
	if len(queries) == 0 {
		fs.Usage()
		return exitUsage
	}
	sf.keepSource = true
	s, err := collect(root, sf)
	if err != nil {
		return fail(err)
	}

	all := candidates(s.files)
	var hits []hit
	seen := map[string]bool{}
	missing := 0
	for _, q := range queries {
		found := findSymbol(all, q)
		if len(found) == 0 {
			reportMissing(all, q, s.label(), sf.includeTests)
			missing++
			continue
		}
		for _, h := range found {
			// the same definition can match several queries ("Get", "DB.Get")
			if k := fmt.Sprintf("%s:%d:%s", h.f.abs, h.sym.Start, h.name); !seen[k] {
				seen[k] = true
				hits = append(hits, h)
			}
		}
	}
	code := writeOut(func(w io.Writer) int {
		for i, h := range hits {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "## %s:%d-%d (%s %s)\n```go\n%s\n```\n",
				h.f.disp, h.sym.Start, h.sym.End, h.sym.Kind, h.name,
				lines(h.f.src, h.sym.Start, h.sym.End))
		}
		return exitOK
	})
	if code == exitOK && missing > 0 {
		return exitFail // found ones are printed; any miss is still a failure
	}
	return code
}

// splitSliceArgs separates symbols from the optional trailing path. Symbols
// may be comma-separated. Without --path, the last of two or more arguments
// is the path when it exists on disk (keeps `slice Name dir` working).
func splitSliceArgs(pos []string, pathFlag string) (queries []string, root string) {
	root = "."
	switch {
	case pathFlag != "":
		root = pathFlag
	case len(pos) >= 2:
		if _, err := os.Stat(pos[len(pos)-1]); err == nil {
			root, pos = pos[len(pos)-1], pos[:len(pos)-1]
		}
	}
	for _, p := range pos {
		for _, q := range strings.Split(p, ",") {
			if q = strings.TrimSpace(q); q != "" {
				queries = append(queries, q)
			}
		}
	}
	return queries, root
}

func candidates(files []*source) []hit {
	var all []hit
	for _, f := range files {
		for _, s := range f.info.Symbols {
			all = append(all, hit{f, s, s.Name})
		}
	}
	return all
}

// findSymbol matches query exactly (qualified or bare name), falling back to
// a case-insensitive match.
func findSymbol(all []hit, query string) []hit {
	match := func(eq func(a, b string) bool) []hit {
		var out []hit
		for _, h := range all {
			short := h.name[strings.LastIndexByte(h.name, '.')+1:]
			if eq(h.name, query) || (!strings.Contains(query, ".") && eq(short, query)) {
				out = append(out, h)
			}
		}
		return out
	}
	if hits := match(func(a, b string) bool { return a == b }); len(hits) > 0 {
		return hits
	}
	return match(strings.EqualFold)
}

func reportMissing(all []hit, query, root string, includeTests bool) {
	fmt.Fprintf(os.Stderr, "ctxmap: symbol %q not found under %s\n", query, root)
	var similar []string
	q := strings.ToLower(query)
	for _, h := range all {
		if len(similar) < 10 && strings.Contains(strings.ToLower(h.name), q) {
			similar = append(similar, h.name+" ("+h.f.disp+")")
		}
	}
	if len(similar) > 0 {
		fmt.Fprintf(os.Stderr, "similar: %s\n", strings.Join(similar, ", "))
	}
	if !includeTests {
		fmt.Fprintln(os.Stderr, "hint: add --include-tests to search test files")
	}
}

// lines returns the 1-based inclusive line range [from, to] of src.
func lines(src []byte, from, to int) string {
	all := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	from = max(from, 1)
	to = min(to, len(all))
	if from > to {
		return ""
	}
	return strings.Join(all[from-1:to], "\n")
}
