package parser

import "testing"

// FuzzParseGo checks that malformed Go is reported as an error, not a panic.
func FuzzParseGo(f *testing.F) {
	f.Add(goSample)
	f.Add("package p\nfunc (")
	f.Fuzz(func(_ *testing.T, src string) {
		_, _ = ParseGo("x.go", []byte(src))
	})
}
