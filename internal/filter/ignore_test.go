package filter

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestIsTestFile(t *testing.T) {
	for name, want := range map[string]bool{
		"a_test.go": true, "a.go": false, "x.spec.ts": true, "x.test.tsx": true,
		"x.test.js": true, "contest.ts": false, "test.ts": false,
	} {
		if IsTestFile(name) != want {
			t.Errorf("IsTestFile(%q) != %v", name, want)
		}
	}
}

func TestWalk(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"main.go", "main_test.go", "x.pb.go", "go.sum",
		"node_modules/a/index.js", "dist/out.js", "web/app.min.js", "web/app.ts",
		"web/app.spec.ts", "gen/code.go", "logs/keep.go", "sub/skip.ts", "sub/keep.ts",
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("# comment\ngen/\n/logs/*.go\n!logs/keep.go\n"), 0o644)
	os.WriteFile(filepath.Join(root, "sub", ".gitignore"), []byte("skip.*\n"), 0o644)

	walk := func(opts Options) string {
		var got []string
		err := Walk(root, opts, func(p string) error {
			rel, _ := filepath.Rel(root, p)
			got = append(got, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(got)
		return strings.Join(got, ",")
	}
	if got, want := walk(Options{}), "logs/keep.go,main.go,sub/keep.ts,web/app.ts"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got, want := walk(Options{IncludeTests: true}), "logs/keep.go,main.go,main_test.go,sub/keep.ts,web/app.spec.ts,web/app.ts"; got != want {
		t.Errorf("tests: got %s\nwant %s", got, want)
	}
	if got, want := walk(Options{NoGitignore: true}), "gen/code.go,logs/keep.go,main.go,sub/keep.ts,sub/skip.ts,web/app.ts"; got != want {
		t.Errorf("no-gitignore: got %s\nwant %s", got, want)
	}
	// --exclude is relative to the root; "!" can re-include a default exclusion
	if got, want := walk(Options{Excludes: []string{"web/", "logs/*.go", "!dist/"}}), "dist/out.js,main.go,sub/keep.ts"; got != want {
		t.Errorf("excludes: got %s\nwant %s", got, want)
	}
	if err := Walk(root, Options{Excludes: []string{"[z-a]"}}, func(string) error { return nil }); err == nil {
		t.Error("invalid --exclude pattern should be an error")
	}
}

func TestCtxmapignore(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"a.go", "mocks/m.go", "build/gen.go", "web/x.ts", "web/keep.ts", "gen.go"} {
		p := filepath.Join(root, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	// .gitignore ignores gen.go; .ctxmapignore re-includes it (applied later),
	// re-includes the default-excluded build/, and drops mocks/ and web/x.ts
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("gen.go\n"), 0o644)
	os.WriteFile(filepath.Join(root, IgnoreFileName), []byte("mocks/\n!build/\n!gen.go\n"), 0o644)
	os.WriteFile(filepath.Join(root, "web", IgnoreFileName), []byte("x.ts\n"), 0o644)

	var got []string
	if err := Walk(root, Options{}, func(p string) error {
		rel, _ := filepath.Rel(root, p)
		got = append(got, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if g, want := strings.Join(got, ","), "a.go,build/gen.go,gen.go,web/keep.ts"; g != want {
		t.Errorf("got  %s\nwant %s", g, want)
	}
}

func TestGitignoreCannotReincludeDefaults(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "vendor", "v.go")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("!vendor/\n"), 0o644)
	n := 0
	if err := Walk(root, Options{}, func(string) error { n++; return nil }); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("vendor/ must stay excluded unless .ctxmapignore/--exclude re-includes it")
	}
}
