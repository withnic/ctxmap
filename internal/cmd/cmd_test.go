package cmd

import (
	"encoding/json"
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
	"go.mod":           "module example.com/app\n\ngo 1.22\n",
	"main.go":          "package main\n\nimport \"example.com/app/store\"\n\nfunc main() { store.New() }\n",
	"store/db.go":      "package store\n\nimport \"example.com/app/store/kv\"\n\n// DB is a store.\ntype DB struct{}\n\n// New makes a DB.\nfunc New() *DB { return &DB{} }\n\nfunc (d *DB) Get(k string) string { return kv.X + k }\n",
	"store/kv/kv.go":   "package kv\n\nconst X = \"x\"\n",
	"web/app.ts":       "import { util } from './lib';\nimport React from 'react';\nexport class App {\n  run(): void { util(); }\n}\n",
	"web/lib/index.ts": "export function util(): number { return 1 }\n",
}

func TestDepsGraph(t *testing.T) {
	root := writeTree(t, fixture)
	g := newDepGraph(mustCollect(t, root))

	var b strings.Builder
	g.writeAll(&b, g.files, false, 0)
	for _, want := range []string{"main.go -> store\n", "store/db.go -> store/kv\n", "web/app.ts -> web/lib/index.ts\n", "3/5 files"} {
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

func TestTSConfigPaths(t *testing.T) {
	root := writeTree(t, map[string]string{
		"tsconfig.base.json": `{
			// shared settings
			"compilerOptions": {
				"baseUrl": ".",
				"paths": { "@/*": ["src/*"], "@ui": ["src/ui/index.ts"], },
			},
		}`,
		"tsconfig.json":     `{ "extends": "./tsconfig.base", /* no overrides */ }`,
		"src/app.ts":        "import { a } from '@/lib/util';\nimport { B } from '@ui';\nimport x from 'react';\nimport y from 'lib/util';\nexport const z = 1;\n",
		"src/lib/util.ts":   "export const a = 1;\n",
		"src/ui/index.ts":   "export const B = 1;\n",
		"src/lib/nested.ts": "export const n = 1;\n",
	})
	g := newDepGraph(mustCollect(t, root))
	for _, f := range g.files {
		if f.disp != "src/app.ts" {
			continue
		}
		// '@/lib/util' via paths, '@ui' via an exact alias, 'lib/util' via
		// baseUrl? no: baseUrl is the root, so 'lib/util' does not exist there.
		got := strings.Join(g.deps[f], ", ")
		if want := "src/lib/util.ts, src/ui/index.ts"; got != want {
			t.Errorf("deps = %q, want %q", got, want)
		}
		return
	}
	t.Fatal("src/app.ts not scanned")
}

func TestStripJSONC(t *testing.T) {
	in := `{"a": "x // not a comment", /* c */ "b": [1, 2,], // tail
}`
	var got struct {
		A string `json:"a"`
		B []int  `json:"b"`
	}
	if err := json.Unmarshal(stripJSONC([]byte(in)), &got); err != nil {
		t.Fatalf("not valid JSON after stripping: %v", err)
	}
	if got.A != "x // not a comment" || len(got.B) != 2 {
		t.Errorf("got %+v", got)
	}
}

func TestFindSymbol(t *testing.T) {
	all := candidates(mustCollect(t, writeTree(t, fixture)).files)
	for query, want := range map[string]string{
		"New":    "New",
		"DB.Get": "DB.Get",
		"Get":    "DB.Get",
		"run":    "App.run",
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
		"web/ 1 files, 1 symbols: App\n",
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
