package cmd

import (
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree creates files (slash paths -> content) under a temp dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func mustCollect(t *testing.T, root string) *scan {
	t.Helper()
	s, err := collect(root, &scanFlags{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

var fixture = map[string]string{
	"go.mod":         "module example.com/app\n\ngo 1.22\n",
	"main.go":        "package main\n\nimport \"example.com/app/store\"\n\nfunc main() { store.New() }\n",
	"store/db.go":    "package store\n\nimport \"example.com/app/store/kv\"\n\n// DB is a store.\ntype DB struct{}\n\n// New makes a DB.\nfunc New() *DB { return &DB{} }\n\nfunc (d *DB) Get(k string) string { return kv.X + k }\n",
	"store/kv/kv.go": "package kv\n\nconst X = \"x\"\n",
	"web/app.go":     "package web\n\nimport (\n\t\"net/http\"\n\n\t\"example.com/app/web/lib\"\n)\n\nfunc Run(_ http.Handler) { lib.Util() }\n",
	"web/lib/lib.go": "package lib\n\nfunc Util() int { return 1 }\n",
}

func TestDepsGraph(t *testing.T) {
	root := writeTree(t, fixture)
	g := newDepGraph(mustCollect(t, root))

	var b strings.Builder
	g.writeAll(&b, g.files, false, 0)
	for _, want := range []string{"main.go -> store\n", "store/db.go -> store/kv\n", "web/app.go -> web/lib\n", "3/5 files"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("deps output missing %q:\n%s", want, b.String())
		}
	}

	for _, target := range []string{"store", "example.com/app/store", filepath.Join(root, "store")} {
		got := g.resolveTarget(target)
		if len(got) != 1 || got[0].disp != "store/db.go" {
			t.Errorf("resolveTarget(%q) = %v", target, got)
			continue
		}
		b.Reset()
		g.writeTarget(&b, target, got, 1)
		want := "depends on (1):\n- store/kv\nreferenced by (1):\n- main.go\n"
		if !strings.HasSuffix(b.String(), want) {
			t.Errorf("writeTarget(%q):\n%s", target, b.String())
		}
	}
}

func TestDepsDepth(t *testing.T) {
	g := newDepGraph(mustCollect(t, writeTree(t, fixture)))
	var b strings.Builder

	g.writeTarget(&b, "kv", g.resolveTarget("kv"), 0)
	if want := "referenced by (2):\n- [1] store/db.go\n- [2] main.go\n"; !strings.HasSuffix(b.String(), want) {
		t.Errorf("reverse depth 0:\n%s", b.String())
	}

	b.Reset()
	g.writeTarget(&b, "main.go", g.resolveTarget("main.go"), 2)
	if want := "depends on (2):\n- [1] store\n- [2] store/kv\n"; !strings.Contains(b.String(), want) {
		t.Errorf("forward depth 2:\n%s", b.String())
	}

	b.Reset()
	g.writeTarget(&b, "main.go", g.resolveTarget("main.go"), 1)
	if !strings.Contains(b.String(), "depends on (1):\n- store\n") {
		t.Errorf("depth 1 should stop at direct deps:\n%s", b.String())
	}
}

func TestCheckDepsFlags(t *testing.T) {
	parse := func(args ...string) error {
		fs := flag.NewFlagSet("deps", flag.ContinueOnError)
		target := fs.String("target", "", "")
		depth := fs.Int("depth", 1, "")
		changed := fs.Bool("changed", false, "")
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		return checkDepsFlags(fs, *target, *depth, *changed)
	}
	if parse("--target", "x", "--depth", "3") != nil {
		t.Error("valid flags rejected")
	}
	for _, bad := range [][]string{{"--depth", "2"}, {"--target", "x", "--depth", "-1"}, {"--target", "x", "--changed"}} {
		if parse(bad...) == nil {
			t.Errorf("%v should be rejected", bad)
		}
	}
}

// TestDepsExternalImports: standard library and third-party imports are not
// local dependencies, nor are packages of another module.
func TestDepsExternalImports(t *testing.T) {
	root := writeTree(t, map[string]string{
		"go.mod":        "module example.com/m\n\ngo 1.22\n",
		"a/a.go":        "package a\n\nimport (\n\t\"fmt\"\n\n\t\"github.com/other/lib\"\n\t\"example.com/m/b\"\n\t\"example.com/mother/x\"\n)\n\nvar _ = fmt.Sprint\n",
		"b/b.go":        "package b\n",
		"nested/go.mod": "module example.com/nested\n\ngo 1.22\n",
		"nested/n/n.go": "package n\n\nimport \"example.com/m/b\"\n",
	})
	g := newDepGraph(mustCollect(t, root))
	for _, f := range g.files {
		got := strings.Join(g.deps[f], ", ")
		switch f.disp {
		case "a/a.go":
			if got != "b" { // not fmt, github.com/..., nor example.com/mother (prefix only)
				t.Errorf("a/a.go deps = %q, want b", got)
			}
		case "nested/n/n.go":
			if got != "" { // its own module is example.com/nested
				t.Errorf("nested module must not resolve example.com/m: %q", got)
			}
		}
	}
}

func TestFindSymbol(t *testing.T) {
	all := candidates(mustCollect(t, writeTree(t, fixture)).files)
	for query, want := range map[string]string{
		"New":    "New",
		"DB.Get": "DB.Get",
		"Get":    "DB.Get",
		"run":    "Run", // case-insensitive fallback
		"new":    "New", // case-insensitive fallback
	} {
		hits := findSymbol(all, query)
		if len(hits) != 1 || hits[0].name != want {
			t.Errorf("findSymbol(%q) = %v, want %s", query, hits, want)
		}
	}
	if hits := findSymbol(all, "Missing"); len(hits) != 0 {
		t.Errorf("unexpected hits: %v", hits)
	}
}

func TestSplitSliceArgs(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		pos      []string
		pathFlag string
		queries  string
		root     string
	}{
		{[]string{"A"}, "", "A", "."},
		{[]string{"A", dir}, "", "A", dir},                         // trailing existing path
		{[]string{"A", "B"}, "", "A,B", "."},                       // B is not a path
		{[]string{"A,B", "C", dir}, "", "A,B,C", dir},              // commas and spaces mix
		{[]string{"A", dir}, "elsewhere", "A," + dir, "elsewhere"}, // --path wins
	}
	for _, c := range cases {
		q, root := splitSliceArgs(c.pos, c.pathFlag)
		if strings.Join(q, ",") != c.queries || root != c.root {
			t.Errorf("splitSliceArgs(%v, %q) = %v, %q", c.pos, c.pathFlag, q, root)
		}
	}
}

func storeFile(t *testing.T) *source {
	t.Helper()
	for _, f := range mustCollect(t, writeTree(t, fixture)).files {
		if f.disp == "store/db.go" {
			return f
		}
	}
	t.Fatal("store/db.go not scanned")
	return nil
}

func TestRenderFile(t *testing.T) {
	f := storeFile(t)
	want := "// DB is a store.\ntype DB struct{} #L5-6\n// New makes a DB.\nfunc New() *DB #L8-9\nfunc (d *DB) Get(k string) string #L11\n"
	if got := renderFile(f.info, mapOpts{}); got != want {
		t.Errorf("renderFile =\n%s\nwant\n%s", got, want)
	}
	if got := renderFile(f.info, mapOpts{noLines: true, noDoc: true}); strings.Contains(got, "#L") || strings.Contains(got, "//") {
		t.Errorf("--no-lines --no-doc still shows refs/docs:\n%s", got)
	}
}

func TestWithLineRef(t *testing.T) {
	if got := withLineRef("type T struct {\n\tA int\n}", 3, 5); got != "type T struct { #L3-5\n\tA int\n}" {
		t.Errorf("multi-line: %q", got)
	}
	if got := withLineRef("func F()", 7, 7); got != "func F() #L7" {
		t.Errorf("single line: %q", got)
	}
}

func TestWriteSummary(t *testing.T) {
	s := mustCollect(t, writeTree(t, fixture))
	var b strings.Builder
	writeSummary(&b, "x", s.files, mapOpts{}, 0)
	for _, want := range []string{
		"# map x --summary (5 files, 7 symbols)\n",
		". 1 files, 1 symbols\n",
		"store/ 1 files, 3 symbols: DB, New\n",
		"web/ 1 files, 1 symbols: Run\n",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("summary missing %q:\n%s", want, b.String())
		}
	}
}

func TestChangedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := writeTree(t, map[string]string{"a.go": "package a\n", "b.go": "package a\n"})
	run := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("add", ".")
	run("commit", "-q", "-m", "init")
	run("checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(root, "b.go"), []byte("package a\n\nfunc B() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("commit", "-q", "-am", "change b")
	if err := os.WriteFile(filepath.Join(root, "c.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	names := func(base string) string {
		set, err := changedFiles(root, base)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, n := range []string{"a.go", "b.go", "c.go"} {
			if set[pathKey(filepath.Join(root, n))] {
				out = append(out, n)
			}
		}
		return strings.Join(out, ",")
	}
	if got := names(""); got != "c.go" { // only the untracked file vs HEAD
		t.Errorf("changed vs HEAD = %s", got)
	}
	if got := names("main"); got != "b.go,c.go" { // committed on the branch + untracked
		t.Errorf("changed vs main = %s", got)
	}
}

// TestPathKeySymlink covers macOS-style roots where the walked path and the
// path git reports differ only by a symlink (/var vs /private/var).
func TestPathKeySymlink(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	if err := os.WriteFile(filepath.Join(real, "a.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if pathKey(filepath.Join(link, "a.go")) != pathKey(filepath.Join(real, "a.go")) {
		t.Errorf("pathKey should resolve symlinks: %s vs %s",
			pathKey(filepath.Join(link, "a.go")), pathKey(filepath.Join(real, "a.go")))
	}
}

func TestParseArgs(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	v := fs.Bool("v", false, "")
	var l stringList
	fs.Var(&l, "e", "")
	pos, err := parseArgs(fs, []string{"a", "-v", "-e", "x", "b", "-e", "y"})
	if err != nil || !*v || strings.Join(pos, ",") != "a,b" || l.String() != "x,y" {
		t.Errorf("pos=%v v=%v l=%v err=%v", pos, *v, l, err)
	}
	if _, err := parseArgs(fs, []string{"-nope"}); flagExit(err) != exitUsage {
		t.Errorf("unknown flag should be a usage error, got %v", err)
	}
}

func TestLines(t *testing.T) {
	src := []byte("a\r\nb\nc\n")
	if got := lines(src, 2, 3); got != "b\nc" {
		t.Errorf("got %q", got)
	}
	if got := lines(src, 5, 9); got != "" {
		t.Errorf("out of range: %q", got)
	}
}

func TestLooksLikePath(t *testing.T) {
	for arg, want := range map[string]bool{
		".": true, "..": true, "./internal": true, "internal/cmd": true, `internal\cmd`: true,
		"map.go": true, "config": false, "cmd": false, "DB.Get": false, "Config": false,
	} {
		if looksLikePath(arg) != want {
			t.Errorf("looksLikePath(%q) = %v, want %v", arg, !want, want)
		}
	}
}

var sliceFixture = map[string]string{
	"go.mod":      "module example.com/s\n\ngo 1.22\n",
	"a/types.go":  "package a\n\n// Server serves.\ntype Server struct{ Addr string }\n",
	"a/routes.go": "package a\n\n// Start starts.\nfunc (s *Server) Start() error { return nil }\n\nfunc (s Server) addr() string { return s.Addr }\n",
	"b/other.go":  "package b\n\ntype Server struct{}\n\nfunc (s *Server) Stop() {}\n",
	"a/parse.go":  "package a\n\nfunc ParseGo() {}\n",
}

func TestWriteSlice(t *testing.T) {
	s, err := collect(writeTree(t, sliceFixture), &scanFlags{keepSource: true})
	if err != nil {
		t.Fatal(err)
	}
	all := candidates(s.files)
	var b strings.Builder

	// a type lists its methods from other files of the same package only
	if code := writeSlice(&b, all, []string{"Server"}, false); code != exitOK {
		t.Errorf("exit = %d", code)
	}
	out := b.String()
	want := "methods on Server (2):\n- a/routes.go:3-4 func (s *Server) Start() error\n- a/routes.go:6-6 func (s Server) addr() string\n"
	if !strings.Contains(out, want) {
		t.Errorf("method list missing:\n%s", out)
	}
	if !strings.Contains(out, "type Server struct{ Addr string }") ||
		strings.Count(out, "methods on Server (1):\n- b/other.go:5-5 func (s *Server) Stop()") != 1 {
		t.Errorf("each Server must list only its own package's methods:\n%s", out)
	}

	// partial match: found ones are printed, the miss is noted on stdout, exit 0
	b.Reset()
	if code := writeSlice(&b, all, []string{"ParseGo", "ParseGoo"}, false); code != exitOK {
		t.Errorf("partial match exit = %d, want %d", code, exitOK)
	}
	out = b.String()
	if !strings.Contains(out, "(func ParseGo)") ||
		!strings.Contains(out, `> [not found] "ParseGoo"`) ||
		!strings.Contains(out, "--include-tests") {
		t.Errorf("partial match output:\n%s", out)
	}

	// total miss: exit 1, still explained on stdout
	b.Reset()
	if code := writeSlice(&b, all, []string{"Nope1", "Nope2"}, true); code != exitFail {
		t.Errorf("total miss exit = %d, want %d", code, exitFail)
	}
	if out := b.String(); strings.Count(out, "> [not found]") != 2 || strings.Contains(out, "--include-tests") {
		t.Errorf("total miss output:\n%s", out)
	}
}

func TestSkipHugeFiles(t *testing.T) {
	root := writeTree(t, map[string]string{
		"go.mod":   "module example.com/h\n\ngo 1.22\n",
		"small.go": "package h\n\nfunc Small() {}\n",
	})
	big := "package h\n\n// " + strings.Repeat("x", maxFileBytes) + "\nfunc Big() {}\n"
	if err := os.WriteFile(filepath.Join(root, "big.go"), []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	s := mustCollect(t, root)
	if len(s.files) != 1 || s.files[0].disp != "small.go" {
		var names []string
		for _, f := range s.files {
			names = append(names, f.disp)
		}
		t.Errorf("files over maxFileBytes must be skipped, got %v", names)
	}
}
