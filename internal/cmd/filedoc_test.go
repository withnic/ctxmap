package cmd

import (
	"strings"
	"testing"
)

var docFixture = map[string]string{
	"go.mod": "module example.com/app\n\ngo 1.22\n",
	// doc.go holds only the package comment; it wins over other files
	"store/doc.go": "// Copyright 2024 Acme.\n\n// Package store keeps things.\n//\n// # Details\n//\n// Long text.\npackage store\n",
	"store/db.go":  "// Package store is also described here.\npackage store\n\nfunc Get() {}\n",
	// no doc.go: the first package comment found describes the directory
	"web/a.go": "package web\n\nfunc A() {}\n",
	"web/b.go": "// Package web serves pages.\npackage web\n\nfunc B() {}\n",
}

func TestMapFileDoc(t *testing.T) {
	s := mustCollect(t, writeTree(t, docFixture))
	var b strings.Builder
	writeMap(&b, "x", s.files, mapOpts{}, 0)
	out := b.String()
	for _, want := range []string{
		"## store/doc.go [store]\n> Package store keeps things.\n", // doc.go is no longer hidden
		"## store/db.go [store]\n> Package store is also described here.\nfunc Get() #L4\n",
		"## web/a.go [web]\nfunc A() #L3\n", // no package comment, no "> " line
	} {
		if !strings.Contains(out, want) {
			t.Errorf("map missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Copyright") {
		t.Errorf("license header must not be shown:\n%s", out)
	}

	b.Reset()
	writeMap(&b, "x", s.files, mapOpts{fullDoc: true}, 0)
	if out := b.String(); !strings.Contains(out, "> Package store keeps things.\n## ") && !strings.Contains(out, "> Package store keeps things.\n\n## ") {
		t.Errorf("--full-doc must stop before the first heading:\n%s", out)
	}

	b.Reset()
	writeMap(&b, "x", s.files, mapOpts{noDoc: true}, 0)
	if out := b.String(); strings.Contains(out, "\n> ") || strings.Contains(out, "store/doc.go") {
		t.Errorf("--no-doc must hide file docs (and doc.go has nothing else to show):\n%s", out)
	}
}

func TestSummaryDirDoc(t *testing.T) {
	s := mustCollect(t, writeTree(t, docFixture))
	var b strings.Builder
	writeSummary(&b, "x", s.files, mapOpts{}, 0)
	out := b.String()
	for _, want := range []string{
		"store/ 2 files, 1 symbols: Get\n  > Package store keeps things.\n", // doc.go preferred
		"web/ 2 files, 2 symbols: A, B\n  > Package web serves pages.\n",    // first package comment
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}

	// a directory and its description are one block for --max-lines
	b.Reset()
	writeSummary(&b, "x", s.files, mapOpts{}, 3)
	if out := b.String(); !strings.Contains(out, "  > Package store keeps things.\n") || strings.Contains(out, "web/") {
		t.Errorf("budget must keep a directory with its description:\n%s", out)
	}
}

func TestFileHeadingBuild(t *testing.T) {
	s := mustCollect(t, writeTree(t, map[string]string{
		"go.mod":         "module example.com/b\n\ngo 1.22\n",
		"p/open_unix.go": "//go:build unix\n\npackage p\n\nfunc open() {}\n",
		"p/open_win.go":  "//go:build windows\n\npackage p\n\nfunc open() {}\n",
		"p/common.go":    "package p\n\nfunc Common() {}\n",
	}))
	var b strings.Builder
	writeMap(&b, "x", s.files, mapOpts{}, 0)
	out := b.String()
	for _, want := range []string{
		"## p/open_unix.go [p] (go:build unix)\n",
		"## p/open_win.go [p] (go:build windows)\n",
		"## p/common.go [p]\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("map missing %q:\n%s", want, out)
		}
	}
}
