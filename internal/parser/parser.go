// Package parser extracts skeletons (signatures, type definitions, doc
// comments) and imports from Go and TypeScript/JavaScript sources.
package parser

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Symbol is one top-level declaration (or a class member).
type Symbol struct {
	Kind      string // func, method, type, const, var, class, interface, enum, namespace, default, member
	Name      string // methods: "Recv.Name"
	Signature string // declaration without body
	Doc       string // doc comment text (comment markers stripped)
	Exported  bool
	Start     int // first line, including the doc comment (1-based)
	End       int // last line (1-based)
	Members   []Symbol
}

// FileInfo is the parse result of a single file.
type FileInfo struct {
	Path    string
	Lang    string // "go" or "ts"
	Package string // Go package name
	Imports []string
	Symbols []Symbol
	// FileDoc describes the file's role: the package comment in Go, the
	// leading description comment in TS/JS (license headers and tool
	// directives skipped). Empty when there is none.
	FileDoc string
}

// ErrUnsupported is returned by ParseFile for unknown file extensions.
var ErrUnsupported = errors.New("unsupported file type")

// ParseFile dispatches on the file extension.
func ParseFile(path string, src []byte) (*FileInfo, error) {
	switch filepath.Ext(path) {
	case ".go":
		return ParseGo(path, src)
	case ".ts", ".tsx", ".js", ".jsx":
		return ParseTS(path, src)
	}
	return nil, fmt.Errorf("%s: %w", path, ErrUnsupported)
}

// Summary returns the first sentence of a doc comment's first paragraph,
// capped at maxSummary bytes.
func Summary(doc string) string {
	var para []string
	for _, l := range strings.Split(strings.TrimSpace(doc), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "@") {
			break
		}
		para = append(para, l)
	}
	s := strings.Join(para, " ")
	for _, sep := range []string{". ", "。"} {
		for from := 0; ; {
			i := strings.Index(s[from:], sep)
			if i < 0 {
				break
			}
			i += from
			if word := s[strings.LastIndexByte(s[:i], ' ')+1 : i]; word == "e.g" || word == "i.e" || word == "(e.g" || word == "(i.e" {
				from = i + len(sep)
				continue
			}
			s = s[:i+len(sep)]
			break
		}
	}
	s = strings.TrimSpace(s)
	if len(s) > maxSummary {
		cut := maxSummary
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return s
}

const maxSummary = 160

// maxIntroLines caps Intro so one huge package comment (fmt's doc.go is
// ~300 lines) cannot use up the output budget on its own.
const maxIntroLines = 15

// Intro returns the introduction of a file or package comment: everything
// before its first heading (a Go doc "# Heading" line), at most
// maxIntroLines lines, without trailing blank lines.
func Intro(doc string) string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(doc), "\n") {
		if strings.HasPrefix(l, "# ") || len(out) == maxIntroLines {
			break
		}
		out = append(out, strings.TrimRight(l, " \t"))
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
