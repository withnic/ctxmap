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
