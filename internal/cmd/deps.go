package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var tsResolveExts = []string{".ts", ".tsx", ".js", ".jsx", ".d.ts"}

// depGraph holds the local dependencies of every scanned file.
type depGraph struct {
	*scan
	deps      map[*source][]string // display keys: Go package dir or TS file
	byKey     map[string][]*source // display key -> files it stands for
	mods      map[string]*goMod    // dir -> nearest go.mod (nil if none)
	tsconfigs map[string]*tsConfig // dir -> nearest tsconfig (nil if none)
}

type goMod struct {
	root, path string
}

// edge is a dependency found at a given distance from the target.
type edge struct {
	name  string
	level int
}

// Deps implements `ctxmap deps [path] [--target X [--depth N]]`.
func Deps(args []string) int {
	fs := newFlagSet("deps", "deps [path] [--target <file|dir|package> [--depth N]] [flags]")
	sf := addScanFlags(fs)
	cf := addChangeFlags(fs)
	target := fs.String("target", "", "show only what the target depends on and what depends on it")
	depth := fs.Int("depth", 1, "with --target: follow dependencies this many levels (0 = unlimited)")
	maxLines := addMaxLinesFlag(fs)
	pos, err := parseArgs(fs, args)
	if err != nil {
		return flagExit(err)
	}
	if err = checkDepsFlags(fs, *target, *depth, cf.changed); err == nil {
		err = checkMaxLines(*maxLines)
	}
	if err != nil {
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
	g := newDepGraph(s)

	if *target == "" {
		changed, err := cf.fileSet(root)
		if err != nil {
			return fail(err)
		}
		return writeOut(func(w io.Writer) int {
			g.writeAll(w, filterFiles(g.files, changed), cf.changed, *maxLines)
			return exitOK
		})
	}
	targets := g.resolveTarget(*target)
	if len(targets) == 0 {
		return fail(fmt.Errorf("target %q not found under %s", *target, s.label()))
	}
	return writeOut(func(w io.Writer) int {
		g.writeTarget(w, *target, targets, *depth)
		return exitOK
	})
}

func checkDepsFlags(fs *flag.FlagSet, target string, depth int, changed bool) error {
	depthSet := false
	fs.Visit(func(f *flag.Flag) { depthSet = depthSet || f.Name == "depth" })
	switch {
	case depth < 0:
		return errors.New("--depth must be >= 0")
	case depthSet && target == "":
		return errors.New("--depth requires --target")
	case changed && target != "":
		return errors.New("--changed cannot be combined with --target")
	}
	return nil
}

func newDepGraph(s *scan) *depGraph {
	g := &depGraph{scan: s, deps: map[*source][]string{}, byKey: map[string][]*source{},
		mods: map[string]*goMod{}, tsconfigs: map[string]*tsConfig{}}
	for _, f := range s.files {
		g.deps[f] = g.localDeps(f)
		k := g.nodeKey(f)
		g.byKey[k] = append(g.byKey[k], f)
	}
	return g
}

// writeAll lists every file with local dependencies, one line per file,
// within the --max-lines budget. The header always reports the full counts.
func (g *depGraph) writeAll(w io.Writer, files []*source, changedOnly bool, maxLines int) {
	var lines []string
	for _, f := range files {
		if d := g.deps[f]; len(d) > 0 {
			lines = append(lines, f.disp+" -> "+strings.Join(d, ", ")+"\n")
		}
	}
	label := g.label()
	if changedOnly {
		label += " --changed"
	}
	fmt.Fprintf(w, "# deps %s (local only; %d/%d files have local deps)\n", label, len(lines), len(files))
	budget := newLineBudget(maxLines)
	for _, l := range lines {
		budget.write(w, l)
	}
	budget.truncated(w, "files", "narrow with --target or a subdirectory path")
}

func (g *depGraph) writeTarget(w io.Writer, query string, targets []*source, depth int) {
	keys := map[string]bool{} // what other files import when they use the target
	var names []string
	for _, t := range targets {
		keys[g.nodeKey(t)] = true
		names = append(names, t.disp)
	}
	header := "# deps --target " + query
	if depth != 1 {
		header += fmt.Sprintf(" --depth %d", depth)
	}
	fmt.Fprintf(w, "%s\ntarget: %s\n", header, strings.Join(names, ", "))
	writeEdges(w, "depends on", g.dependsOn(targets, keys, depth), depth != 1)
	writeEdges(w, "referenced by", g.referencedBy(targets, keys, depth), depth != 1)
}

// dependsOn walks dependencies breadth-first from the targets.
func (g *depGraph) dependsOn(targets []*source, keys map[string]bool, depth int) []edge {
	seen := map[string]bool{}
	for k := range keys {
		seen[k] = true
	}
	var out []edge
	frontier := targets
	for level := 1; len(frontier) > 0 && (depth == 0 || level <= depth); level++ {
		var next []*source
		for _, f := range frontier {
			for _, d := range g.deps[f] {
				if !seen[d] {
					seen[d] = true
					out = append(out, edge{d, level})
					next = append(next, g.byKey[d]...)
				}
			}
		}
		frontier = next
	}
	return sortEdges(out)
}

// referencedBy walks reverse dependencies breadth-first from the targets.
func (g *depGraph) referencedBy(targets []*source, keys map[string]bool, depth int) []edge {
	seenFile := map[*source]bool{}
	for _, t := range targets {
		seenFile[t] = true
	}
	seenKey := map[string]bool{}
	for k := range keys {
		seenKey[k] = true
	}
	var out []edge
	cur := keys
	for level := 1; len(cur) > 0 && (depth == 0 || level <= depth); level++ {
		next := map[string]bool{}
		for _, f := range g.files {
			if seenFile[f] || !importsAny(g.deps[f], cur) {
				continue
			}
			seenFile[f] = true
			out = append(out, edge{f.disp, level})
			if k := g.nodeKey(f); !seenKey[k] {
				seenKey[k], next[k] = true, true
			}
		}
		cur = next
	}
	return sortEdges(out)
}

func importsAny(deps []string, keys map[string]bool) bool {
	for _, d := range deps {
		if keys[d] {
			return true
		}
	}
	return false
}

func sortEdges(e []edge) []edge {
	sort.SliceStable(e, func(i, j int) bool {
		if e[i].level != e[j].level {
			return e[i].level < e[j].level
		}
		return e[i].name < e[j].name
	})
	return e
}

func writeEdges(w io.Writer, title string, edges []edge, showLevel bool) {
	fmt.Fprintf(w, "%s (%d):\n", title, len(edges))
	for _, e := range edges {
		if showLevel {
			fmt.Fprintf(w, "- [%d] %s\n", e.level, e.name)
		} else {
			fmt.Fprintf(w, "- %s\n", e.name)
		}
	}
}

// uniq returns the sorted distinct elements of s.
func uniq(s []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// nodeKey is how a file appears as a dependency of other files.
func (g *depGraph) nodeKey(f *source) string {
	if f.info.Lang == "go" {
		return g.rel(filepath.Dir(f.abs))
	}
	return f.disp
}

func (g *depGraph) localDeps(f *source) []string {
	var out []string
	dir := filepath.Dir(f.abs)
	for _, imp := range f.info.Imports {
		if f.info.Lang == "go" {
			mod := g.findMod(dir)
			if mod == nil || (imp != mod.path && !strings.HasPrefix(imp, mod.path+"/")) {
				continue
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(imp, mod.path), "/")
			out = append(out, g.rel(filepath.Join(mod.root, filepath.FromSlash(rel))))
			continue
		}
		if isRelativeImport(imp) {
			p := filepath.Join(dir, filepath.FromSlash(imp))
			if abs, ok := resolveFile(p); ok {
				out = append(out, g.rel(abs))
			} else {
				out = append(out, g.rel(p)+" (unresolved)")
			}
			continue
		}
		// tsconfig/jsconfig "paths" aliases and baseUrl; anything that does
		// not resolve to a file is a package import.
		if c := g.tsConfigFor(dir); c != nil {
			for _, cand := range c.candidates(imp) {
				if abs, ok := resolveFile(cand); ok {
					out = append(out, g.rel(abs))
					break
				}
			}
		}
	}
	return uniq(out)
}

func isRelativeImport(imp string) bool {
	return imp == "." || imp == ".." || strings.HasPrefix(imp, "./") || strings.HasPrefix(imp, "../")
}

// resolveFile maps an import path (without or with extension) to an existing
// file, trying TS/JS extensions and index files.
func resolveFile(p string) (string, bool) {
	if fileExists(p) {
		return p, true
	}
	// ESM-style "./foo.js" that actually points at foo.ts
	base := strings.TrimSuffix(p, filepath.Ext(p))
	for _, cand := range []string{p, base} {
		for _, ext := range tsResolveExts {
			if fileExists(cand + ext) {
				return cand + ext, true
			}
		}
	}
	for _, ext := range tsResolveExts {
		if q := filepath.Join(p, "index"+ext); fileExists(q) {
			return q, true
		}
	}
	return "", false
}

// findMod returns the nearest go.mod at or above dir, cached per directory.
func (g *depGraph) findMod(dir string) *goMod {
	if m, ok := g.mods[dir]; ok {
		return m
	}
	var m *goMod
	if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			if f := strings.Fields(l); len(f) >= 2 && f[0] == "module" {
				m = &goMod{root: dir, path: strings.Trim(f[1], `"`)}
				break
			}
		}
	} else if parent := filepath.Dir(dir); parent != dir {
		m = g.findMod(parent)
	}
	g.mods[dir] = m
	return m
}

// resolveTarget matches --target against files, directories, Go import paths
// and finally loose names (package name, file name without extension).
func (g *depGraph) resolveTarget(t string) []*source {
	if out := g.targetByPath(t); len(out) > 0 {
		return out
	}
	norm := strings.TrimSuffix(filepath.ToSlash(t), "/")
	if out := g.targetByImportPath(norm); len(out) > 0 {
		return out
	}
	var out []*source
	for _, f := range g.files {
		noExt := strings.TrimSuffix(f.disp, filepath.Ext(f.disp))
		dir := filepath.ToSlash(filepath.Dir(f.disp))
		goPkg := f.info.Lang == "go" && (f.info.Package == norm || dir == norm || strings.HasSuffix(dir, "/"+norm))
		if goPkg || f.disp == norm || noExt == norm || filepath.Base(noExt) == norm || strings.HasSuffix(noExt, "/"+norm) {
			out = append(out, f)
		}
	}
	return out
}

// targetByPath matches an existing file, or the files directly in a directory.
func (g *depGraph) targetByPath(t string) []*source {
	abs, err := filepath.Abs(t)
	if err != nil {
		return nil
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil
	}
	var out []*source
	for _, f := range g.files {
		if (!st.IsDir() && f.abs == abs) || (st.IsDir() && filepath.Dir(f.abs) == abs) {
			out = append(out, f)
		}
	}
	return out
}

// targetByImportPath matches Go files whose package import path is ip.
func (g *depGraph) targetByImportPath(ip string) []*source {
	var out []*source
	for _, f := range g.files {
		if f.info.Lang != "go" {
			continue
		}
		dir := filepath.Dir(f.abs)
		mod := g.findMod(dir)
		if mod == nil {
			continue
		}
		rel, err := filepath.Rel(mod.root, dir)
		if err != nil {
			continue
		}
		path := mod.path
		if rel != "." {
			path += "/" + filepath.ToSlash(rel)
		}
		if path == ip {
			out = append(out, f)
		}
	}
	return out
}
