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

	return writeOut(func(w io.Writer) int {
		return writeSlice(w, candidates(s.files), queries, sf.includeTests)
	})
}

// writeSlice prints the definition of every symbol that matches a query.
// A type is followed by the list of its methods in the whole package.
// Queries without a match are noted at the end of stdout (where an agent
// reads them), with suggestions; the result is exitFail only when nothing
// matched at all.
func writeSlice(w io.Writer, all []hit, queries []string, includeTests bool) int {
	var hits []hit
	var notes []string
	seen := map[string]bool{}
	for _, q := range queries {
		found := findSymbol(all, q)
		if len(found) == 0 {
			notes = append(notes, notFoundNote(all, q))
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
	if len(notes) > 0 && !includeTests {
		notes = append(notes, "> hint: test files are not searched; add --include-tests to include them")
	}

	methods := methodsByType(all)
	for i, h := range hits {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "## %s:%d-%d (%s %s)\n```go\n%s\n```\n",
			h.f.disp, h.sym.Start, h.sym.End, h.sym.Kind, h.name,
			lines(h.f.src, h.sym.Start, h.sym.End))
		if h.sym.Kind == "type" {
			writeMethods(w, h.name, methods[typeKey(h.f, h.name)])
		}
	}
	if len(notes) > 0 {
		if len(hits) > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, strings.Join(notes, "\n"))
	}
	if len(hits) == 0 {
		return exitFail
	}
	return exitOK
}

// typeKey identifies a type within its package (directory).
func typeKey(f *source, typeName string) string {
	return f.dir() + "\x00" + typeName
}

// methodsByType groups the methods of every scanned file by receiver type,
// so `slice <Type>` can list methods declared in any file of the package.
func methodsByType(all []hit) map[string][]hit {
	out := map[string][]hit{}
	for _, h := range all {
		if h.sym.Kind != "method" {
			continue
		}
		recv := h.name[:strings.IndexByte(h.name, '.')]
		k := typeKey(h.f, recv)
		out[k] = append(out[k], h)
	}
	return out
}

// writeMethods lists method signatures and locations (no bodies).
func writeMethods(w io.Writer, typeName string, methods []hit) {
	if len(methods) == 0 {
		return
	}
	fmt.Fprintf(w, "methods on %s (%d):\n", typeName, len(methods))
	for _, m := range methods {
		sig := strings.Join(strings.Fields(m.sym.Signature), " ") // one line
		fmt.Fprintf(w, "- %s:%d-%d %s\n", m.f.disp, m.sym.Start, m.sym.End, sig)
	}
}

// splitSliceArgs separates symbols from the optional trailing path. Symbols
// may be comma-separated. Without --path, the last of two or more arguments
// is the path only when it clearly is one (see looksLikePath) and exists, so
// a symbol that shares its name with a directory (`slice Config config`) is
// still searched as a symbol.
func splitSliceArgs(pos []string, pathFlag string) (queries []string, root string) {
	root = "."
	switch {
	case pathFlag != "":
		root = pathFlag
	case len(pos) >= 2:
		last := pos[len(pos)-1]
		if _, err := os.Stat(last); err == nil && looksLikePath(last) {
			root, pos = last, pos[:len(pos)-1]
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

// looksLikePath reports whether a positional argument is clearly a path
// rather than a symbol: ".", "..", anything with a path separator, or a
// .go file name.
func looksLikePath(s string) bool {
	return s == "." || s == ".." || strings.ContainsAny(s, `/\`) || strings.HasSuffix(s, ".go")
}

// notFoundNote is the line printed for a symbol that was not found, with up
// to five similar names as suggestions.
func notFoundNote(all []hit, query string) string {
	var similar []string
	q := strings.ToLower(query)
	for _, h := range all {
		if len(similar) < 5 && strings.Contains(strings.ToLower(h.name), q) {
			similar = append(similar, h.name+" ("+h.f.disp+")")
		}
	}
	note := fmt.Sprintf("> [not found] %q", query)
	if len(similar) > 0 {
		note += " (did you mean: " + strings.Join(similar, ", ") + "?)"
	}
	return note
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
