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
		"a_test.go": true, "a.go": false, "test.go": false, "contest.go": false,
		"x.test.ts": false, // not Go
	} {
		if IsTestFile(name) != want {
			t.Errorf("IsTestFile(%q) != %v", name, want)
		}
	}
}

func TestWalk(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"main.go", "main_test.go", "x.pb.go", "go.sum", "web/app.ts",
		"node_modules/a/a.go", "vendor/v/v.go", "testdata/t.go", "build/b.go",
		"gen/code.go", "logs/keep.go", "sub/skip.go", "sub/keep.go",
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
	// build/ is a normal Go package directory; only Go files are listed
	if got, want := walk(Options{}), "build/b.go,logs/keep.go,main.go,sub/keep.go"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got, want := walk(Options{IncludeTests: true}), "build/b.go,logs/keep.go,main.go,main_test.go,sub/keep.go"; got != want {
		t.Errorf("tests: got %s\nwant %s", got, want)
	}
	if got, want := walk(Options{NoGitignore: true}), "build/b.go,gen/code.go,logs/keep.go,main.go,sub/keep.go,sub/skip.go"; got != want {
		t.Errorf("no-gitignore: got %s\nwant %s", got, want)
	}
	// --exclude is relative to the root; "!" can re-include a default exclusion
	if got, want := walk(Options{Excludes: []string{"build/", "logs/*.go", "!testdata/"}}), "main.go,sub/keep.go,testdata/t.go"; got != want {
		t.Errorf("excludes: got %s\nwant %s", got, want)
	}
	if err := Walk(root, Options{Excludes: []string{"[z-a]"}}, func(string) error { return nil }); err == nil {
		t.Error("invalid --exclude pattern should be an error")
	}
}

func TestCtxmapignore(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"a.go", "mocks/m.go", "testdata/fixture.go", "web/x.go", "web/keep.go", "gen.go"} {
		p := filepath.Join(root, filepath.FromSlash(f))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	// .gitignore ignores gen.go; .ctxmapignore re-includes it (applied later),
	// re-includes the default-excluded testdata/, and drops mocks/ and web/x.go
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("gen.go\n"), 0o644)
	os.WriteFile(filepath.Join(root, IgnoreFileName), []byte("mocks/\n!testdata/\n!gen.go\n"), 0o644)
	os.WriteFile(filepath.Join(root, "web", IgnoreFileName), []byte("x.go\n"), 0o644)

	var got []string
	if err := Walk(root, Options{}, func(p string) error {
		rel, _ := filepath.Rel(root, p)
		got = append(got, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if g, want := strings.Join(got, ","), "a.go,gen.go,testdata/fixture.go,web/keep.go"; g != want {
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
