package parser

import "testing"

// FuzzParseTS checks that arbitrary input never panics the lightweight
// TS/JS parser (unbalanced braces, unterminated strings/templates/regexes).
func FuzzParseTS(f *testing.F) {
	for _, s := range []string{
		tsSample,
		"export const a = `${`${x}`}`;",
		"export function f(a: {b: string}): {c: number} {",
		"export class A { m() { return /[}]/ } }",
		"/** unterminated",
		"export type T = | 'a' |",
		"}}}{{{ export default",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if _, err := ParseTS("x.tsx", []byte(src)); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzParseGo checks that malformed Go is reported as an error, not a panic.
func FuzzParseGo(f *testing.F) {
	f.Add(goSample)
	f.Add("package p\nfunc (")
	f.Fuzz(func(_ *testing.T, src string) {
		_, _ = ParseGo("x.go", []byte(src))
	})
}
