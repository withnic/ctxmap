// Package filter decides which files ctxmap scans: default exclusions,
// test-file detection, and simplified .gitignore / .ctxmapignore matching.
package filter

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultIgnoreDirs are directory names that are never descended into:
// VCS metadata, vendored modules, testdata (ignored by the go tool too),
// a frontend's node_modules and Claude Code's own directory.
var DefaultIgnoreDirs = map[string]bool{
	".git": true, "vendor": true, "testdata": true, "node_modules": true, ".claude": true,
}

// DefaultIgnoreFiles are file name patterns that are never scanned
// (generated protobuf code).
var DefaultIgnoreFiles = []string{"*.pb.go"}

// IsTestFile reports whether name is a Go test file.
func IsTestFile(name string) bool {
	return strings.HasSuffix(name, "_test.go")
}

// IsSourceFile reports whether name is a Go source file.
func IsSourceFile(name string) bool {
	return filepath.Ext(name) == ".go"
}

// IsIgnoredFile reports whether name matches DefaultIgnoreFiles.
func IsIgnoredFile(name string) bool {
	for _, p := range DefaultIgnoreFiles {
		if ok, _ := filepath.Match(p, name); ok {
			return true
		}
	}
	return false
}

// IgnoreFileName is the per-directory ctxmap-specific ignore file. It uses
// .gitignore syntax and is applied after .gitignore, so its rules (including
// "!pattern" negations) win over both .gitignore and the default exclusions.
const IgnoreFileName = ".ctxmapignore"

// Options controls Walk.
type Options struct {
	IncludeTests bool
	// Excludes are extra .gitignore-style patterns, relative to the walk root.
	// They are applied last and override every other rule.
	Excludes []string
	// NoGitignore disables .gitignore handling (.ctxmapignore still applies).
	NoGitignore bool
	// OnError receives non-fatal errors (unreadable directories, ...).
	// Walk skips the offending entry and continues. Nil ignores them.
	OnError func(error)
}

// Walk calls fn with the absolute path of every source file under root that
// survives the exclusion rules, applied in this order (later wins):
// defaults, .gitignore, .ctxmapignore, Options.Excludes. If root is a file it
// is passed to fn directly (explicitly named files bypass the filters).
func Walk(root string, opts Options, fn func(path string) error) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		if !IsSourceFile(absRoot) {
			return fmt.Errorf("%s: unsupported file type", root)
		}
		return fn(absRoot)
	}

	w, err := newWalker(absRoot, opts)
	if err != nil {
		return err
	}
	return filepath.WalkDir(absRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if opts.OnError != nil {
				opts.OnError(err)
			}
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != absRoot && w.skip(p, true, DefaultIgnoreDirs[name]) {
				return filepath.SkipDir
			}
			w.load(p)
			return nil
		}
		if !d.Type().IsRegular() || !IsSourceFile(name) {
			return nil
		}
		if !opts.IncludeTests && IsTestFile(name) {
			return nil
		}
		if w.skip(p, false, IsIgnoredFile(name)) {
			return nil
		}
		return fn(p)
	})
}

// walker layers the rule sources in precedence order.
type walker struct {
	git *matcher // nil when disabled
	ctx *matcher
	cli *matcher
}

func newWalker(absRoot string, opts Options) (*walker, error) {
	w := &walker{ctx: &matcher{file: IgnoreFileName}, cli: &matcher{}}
	if !opts.NoGitignore {
		w.git = &matcher{file: ".gitignore"}
	}
	var rules []rule
	for _, pat := range opts.Excludes {
		r, ok, err := compileRule(pat)
		if err != nil {
			return nil, fmt.Errorf("--exclude %q: %w", pat, err)
		}
		if ok {
			rules = append(rules, r)
		}
	}
	if len(rules) > 0 {
		w.cli.sets = []ruleSet{{base: filepath.ToSlash(absRoot), rules: rules}}
	}
	for _, dir := range ancestors(absRoot) {
		w.load(dir)
	}
	return w, nil
}

func (w *walker) load(dir string) {
	if w.git != nil {
		w.git.load(dir)
	}
	w.ctx.load(dir)
}

// skip decides whether path is excluded. .gitignore may add exclusions but
// cannot re-include a default exclusion; .ctxmapignore and --exclude can.
func (w *walker) skip(path string, isDir, defaultIgnored bool) bool {
	ignored := defaultIgnored
	if !ignored && w.git != nil {
		if ig, ok := w.git.match(path, isDir); ok {
			ignored = ig
		}
	}
	for _, m := range []*matcher{w.ctx, w.cli} {
		if ig, ok := m.match(path, isDir); ok {
			ignored = ig
		}
	}
	return ignored
}

// ancestors returns the parents of root up to the repository root (the first
// directory containing .git), outermost first. Outside a repository it
// returns nothing, so only rules under root apply.
func ancestors(root string) []string {
	var dirs []string
	for dir := root; ; {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
		dirs = append([]string{dir}, dirs...)
	}
	return dirs
}

// ---- .gitignore-style matching (simplified) ----

type rule struct {
	re      *regexp.Regexp
	negate  bool
	dirOnly bool
}

type ruleSet struct {
	base  string // absolute, slash-separated directory the rules are relative to
	rules []rule
}

// matcher holds rule sets loaded from files named file (if any), shallower
// directories first.
type matcher struct {
	file string
	sets []ruleSet
}

func (m *matcher) load(dir string) {
	if m.file == "" {
		return
	}
	if rules := parseIgnoreFile(filepath.Join(dir, m.file)); len(rules) > 0 {
		m.sets = append(m.sets, ruleSet{base: filepath.ToSlash(dir), rules: rules})
	}
}

// match reports whether the last matching rule ignores path, and whether
// any rule matched at all.
func (m *matcher) match(path string, isDir bool) (ignored, matched bool) {
	p := filepath.ToSlash(path)
	for _, s := range m.sets {
		if !strings.HasPrefix(p, s.base+"/") {
			continue
		}
		rel := p[len(s.base)+1:]
		for _, r := range s.rules {
			if r.dirOnly && !isDir {
				continue
			}
			if r.re.MatchString(rel) {
				ignored, matched = !r.negate, true
			}
		}
	}
	return ignored, matched
}

// parseIgnoreFile reads a .gitignore-style file. A missing file yields no
// rules; invalid patterns are skipped.
func parseIgnoreFile(path string) []rule {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var rules []rule
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if r, ok, err := compileRule(sc.Text()); ok && err == nil {
			rules = append(rules, r)
		}
	}
	return rules
}

// compileRule compiles one .gitignore line. ok is false for blank lines and
// comments.
func compileRule(line string) (r rule, ok bool, err error) {
	line = strings.TrimRight(line, " \t\r")
	if line == "" || strings.HasPrefix(line, "#") {
		return rule{}, false, nil
	}
	if strings.HasPrefix(line, "!") {
		r.negate = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimRight(line, "/")
	}
	anchored := strings.Contains(line, "/")
	line = strings.TrimPrefix(line, "/")
	if line == "" {
		return rule{}, false, nil
	}
	expr := globToRegex(line)
	if anchored {
		expr = "^" + expr + "$"
	} else {
		expr = "^(?:.*/)?" + expr + "$"
	}
	r.re, err = regexp.Compile(expr)
	if err != nil {
		return rule{}, false, err
	}
	return r, true, nil
}

func globToRegex(g string) string {
	var b strings.Builder
	for i := 0; i < len(g); i++ {
		c := g[i]
		switch c {
		case '*':
			if i+1 < len(g) && g[i+1] == '*' {
				if i+2 < len(g) && g[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 2
				} else {
					b.WriteString(".*")
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '[':
			j := strings.IndexByte(g[i:], ']')
			if j > 1 {
				cls := g[i+1 : i+j]
				if strings.HasPrefix(cls, "!") {
					cls = "^" + cls[1:]
				}
				b.WriteString("[" + strings.ReplaceAll(cls, `\`, `\\`) + "]")
				i += j
			} else {
				b.WriteString(`\[`)
			}
		case '\\':
			if i+1 < len(g) {
				b.WriteString(regexp.QuoteMeta(string(g[i+1])))
				i++
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}
