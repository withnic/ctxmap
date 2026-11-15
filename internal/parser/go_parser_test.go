package parser

import "testing"

const goSample = `package demo

import (
	"fmt"
	"example.com/m/sub"
)

// Server serves.
type Server struct {
	Addr    string // listen address
	handler func() error
}

// Run starts the server.
// It blocks.
func (s *Server) Run(ctx string) error {
	fmt.Println(sub.X)
	return nil
}

func helper[T any](v T) T { return v }

const (
	ModeA = iota
	modeB
)
`

func TestParseGo(t *testing.T) {
	fi, err := ParseGo("demo.go", []byte(goSample))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Package != "demo" || len(fi.Imports) != 2 || fi.Imports[1] != "example.com/m/sub" {
		t.Errorf("package/imports: %s %v", fi.Package, fi.Imports)
	}
	want := map[string]string{
		"Server":     "type Server struct {\n\tAddr string\n\thandler func() error\n}",
		"Server.Run": "func (s *Server) Run(ctx string) error",
		"helper":     "func helper[T any](v T) T",
		"ModeA":      "ModeA",
	}
	got := map[string]Symbol{}
	for _, s := range fi.Symbols {
		got[s.Name] = s
	}
	for name, sig := range want {
		if got[name].Signature != sig {
			t.Errorf("%s: sig = %q, want %q", name, got[name].Signature, sig)
		}
	}
	run := got["Server.Run"]
	if run.Kind != "method" || !run.Exported || run.Start != 14 || run.End != 19 || Summary(run.Doc) != "Run starts the server." {
		t.Errorf("Run = %+v", run)
	}
	if got["modeB"].Exported {
		t.Error("modeB should be unexported")
	}
}

func TestGoFileDoc(t *testing.T) {
	src := "// Copyright 2024 Acme. All rights reserved.\n\n// Package demo does demo things.\n//\n// # Usage\n//\n// More text.\npackage demo\n"
	fi, err := ParseGo("doc.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Package demo does demo things.\n\n# Usage\n\nMore text."; fi.FileDoc != want {
		t.Errorf("FileDoc = %q, want %q (license header must not be included)", fi.FileDoc, want)
	}
	if Intro(fi.FileDoc) != "Package demo does demo things." {
		t.Errorf("Intro should stop before the first heading: %q", Intro(fi.FileDoc))
	}
	noDoc, _ := ParseGo("a.go", []byte("// Copyright 2024 Acme.\n\npackage demo\n"))
	if noDoc.FileDoc != "" {
		t.Errorf("license header is not a package doc: %q", noDoc.FileDoc)
	}
}

func TestIntroCap(t *testing.T) {
	long := ""
	for i := 0; i < 40; i++ {
		long += "line\n"
	}
	if got := Intro(long); len(got) != len("line")*maxIntroLines+maxIntroLines-1 {
		t.Errorf("Intro should keep at most %d lines, got %q", maxIntroLines, got)
	}
}
