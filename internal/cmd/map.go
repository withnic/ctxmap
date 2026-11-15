package cmd

import (
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/withnic/ctxmap/internal/parser"
)

type mapOpts struct {
	exportedOnly, fullDoc, noDoc, noLines bool
}

// maxSummaryNames caps the symbol names listed per directory in --summary.
const maxSummaryNames = 8

// Map implements `ctxmap map [path]`: a compact Markdown skeleton of the code.
func Map(args []string) int {
	fs := newFlagSet("map", "map [path] [flags]")
	sf := addScanFlags(fs)
	cf := addChangeFlags(fs)
	var o mapOpts
	summary := fs.Bool("summary", false, "one line per directory (file/symbol counts, main exported names) instead of signatures")
	fs.BoolVar(&o.exportedOnly, "exported", false, "show exported symbols only")
	fs.BoolVar(&o.fullDoc, "full-doc", false, "print whole doc comments instead of their first sentence")
	fs.BoolVar(&o.noDoc, "no-doc", false, "omit doc comments")
	fs.BoolVar(&o.noLines, "no-lines", false, "omit the #Lstart-end line references")
	maxLines := addMaxLinesFlag(fs)
	pos, err := parseArgs(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if err = checkMaxLines(*maxLines); err != nil {
		fmt.Fprintln(os.Stderr, "ctxmap:", err)
		return exitUsage
	}
	root := "."
	if len(pos) > 0 {
		root = pos[0]
	}
	s, err := collect(root, sf)
	if err != nil {
		return fail(err)
	}
	changed, err := cf.fileSet(root)
	if err != nil {
		return fail(err)
	}
	files := filterFiles(s.files, changed)

	label := s.label()
	if cf.changed {
		label += " --changed"
	}
	return writeOut(func(w io.Writer) int {
		if *summary {
			writeSummary(w, label, files, o, *maxLines)
			return exitOK
		}
		writeMap(w, label, files, o, *maxLines)
		return exitOK
	})
}

// fileDocText is how much of a file/package comment map shows: the first
// sentence by default, the introduction (up to the first heading) with
// --full-doc, nothing with --no-doc.
func fileDocText(doc string, o mapOpts) string {
	switch {
	case o.noDoc || doc == "":
		return ""
	case o.fullDoc:
		return parser.Intro(doc)
	}
	return parser.Summary(doc)
}

// dirDoc picks the description of a directory (package) for --summary: the
// package comment in doc.go if there is one, otherwise the first package
// comment found.
func dirDoc(files []*source) string {
	best := ""
	for _, f := range files {
		if f.info.FileDoc == "" {
			continue
		}
		if path.Base(f.disp) == "doc.go" {
			return f.info.FileDoc
		}
		if best == "" {
			best = f.info.FileDoc
		}
	}
	return best
}

// writeMap prints the skeleton of every file, one block per file, within
// the --max-lines budget. The header always reports the full file count.
func writeMap(w io.Writer, label string, files []*source, o mapOpts, maxLines int) {
	fmt.Fprintf(w, "# map %s (%d files)\n", label, len(files))
	budget := newLineBudget(maxLines)
	for _, f := range files {
		body := renderFile(f.info, o)
		if body == "" {
			continue
		}
		budget.write(w, "\n## "+f.disp+" ["+f.info.Package+"]\n"+body)
	}
	budget.truncated(w, "files", "narrow with 'ctxmap map --summary' or a subdirectory path")
}

// visible reports whether map shows s: every func/method/type unless
// --exported, but only exported consts/vars (listed by name).
func visible(s parser.Symbol, o mapOpts) bool {
	if !o.exportedOnly && s.Kind != "const" && s.Kind != "var" {
		return true // unexported funcs/types are part of the skeleton
	}
	return s.Exported
}

// renderFile renders the skeleton of one file; empty when nothing is shown.
func renderFile(fi *parser.FileInfo, o mapOpts) string {
	var b strings.Builder
	writeDoc := func(indent, doc string) {
		switch {
		case o.noDoc:
			return
		case o.fullDoc:
			doc = strings.TrimSpace(doc)
		default:
			doc = parser.Summary(doc)
		}
		if doc == "" {
			return
		}
		for _, l := range strings.Split(doc, "\n") {
			fmt.Fprintf(&b, "%s// %s\n", indent, l)
		}
	}
	writeSig := func(indent string, s parser.Symbol) {
		sig := s.Signature
		if !o.noLines {
			sig = withLineRef(sig, s.Start, s.End)
		}
		b.WriteString(indent + strings.ReplaceAll(sig, "\n", "\n"+indent) + "\n")
	}

	// the file's role (its package comment, e.g. from doc.go) right under
	// the heading, marked "> " to tell it apart from declaration docs
	if doc := fileDocText(fi.FileDoc, o); doc != "" {
		for _, l := range strings.Split(doc, "\n") {
			b.WriteString(strings.TrimRight("> "+l, " ") + "\n")
		}
	}

	var consts, vars []string
	for _, s := range fi.Symbols {
		if !visible(s, o) {
			continue
		}
		if s.Kind == "const" || s.Kind == "var" {
			// consts/vars are listed by name only, one line per kind
			if s.Kind == "const" {
				consts = append(consts, s.Name)
			} else {
				vars = append(vars, s.Name)
			}
			continue
		}
		writeDoc("", s.Doc)
		writeSig("", s)
	}
	if len(consts) > 0 {
		b.WriteString("const " + strings.Join(consts, ", ") + "\n")
	}
	if len(vars) > 0 {
		b.WriteString("var " + strings.Join(vars, ", ") + "\n")
	}
	return b.String()
}

// withLineRef appends " #Lstart-end" to the first line of sig, so a reader
// can fetch exactly that range (doc comment included).
func withLineRef(sig string, start, end int) string {
	ref := fmt.Sprintf(" #L%d", start)
	if end > start {
		ref = fmt.Sprintf(" #L%d-%d", start, end)
	}
	if i := strings.IndexByte(sig, '\n'); i >= 0 {
		return sig[:i] + ref + sig[i:]
	}
	return sig + ref
}

// writeSummary prints one line per directory: file and symbol counts plus
// the first exported names, as an entry point for large repositories.
func writeSummary(w io.Writer, label string, files []*source, o mapOpts, maxLines int) {
	type dirStat struct {
		files, syms int
		names       []string
		members     []*source
	}
	stats := map[string]*dirStat{}
	var order []string
	total := 0
	for _, f := range files {
		dir := path.Dir(f.disp) + "/"
		if dir == "./" {
			dir = "."
		}
		st := stats[dir]
		if st == nil {
			st = &dirStat{}
			stats[dir] = st
			order = append(order, dir)
		}
		st.files++
		st.members = append(st.members, f)
		for _, s := range f.info.Symbols {
			if !visible(s, o) {
				continue
			}
			st.syms++
			total++
			if s.Exported && s.Kind != "method" && len(st.names) <= maxSummaryNames && !slices.Contains(st.names, s.Name) {
				st.names = append(st.names, s.Name) // overloads share a name
			}
		}
	}
	sort.Slice(order, func(i, j int) bool { // "." first, then alphabetical
		return order[i] == "." || (order[j] != "." && order[i] < order[j])
	})
	fmt.Fprintf(w, "# map %s --summary (%d files, %d symbols)\n", label, len(files), total)
	budget := newLineBudget(maxLines)
	for _, dir := range order {
		st := stats[dir]
		line := fmt.Sprintf("%s %d files, %d symbols", dir, st.files, st.syms)
		if names := st.names; len(names) > 0 {
			more := ""
			if len(names) > maxSummaryNames {
				names, more = names[:maxSummaryNames], ", …"
			}
			line += ": " + strings.Join(names, ", ") + more
		}
		line += "\n"
		// the package/directory description, always one sentence here
		if doc := dirDoc(st.members); doc != "" && !o.noDoc {
			line += "  > " + parser.Summary(doc) + "\n"
		}
		budget.write(w, line) // a directory and its description stay together
	}
	budget.truncated(w, "directories", "narrow with a subdirectory path")
}
