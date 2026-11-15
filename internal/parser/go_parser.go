package parser

import (
	"bytes"
	"go/ast"
	"go/build/constraint"
	goparser "go/parser"
	"go/printer"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// ParseGo parses a Go file with go/parser and returns its skeleton.
func ParseGo(path string, src []byte) (*FileInfo, error) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, path, src, goparser.ParseComments)
	if err != nil {
		return nil, err
	}
	// f.Doc is the comment directly above the package clause, so a license
	// header separated by a blank line is not picked up.
	fi := &FileInfo{Path: path, Package: f.Name.Name, FileDoc: strings.TrimSpace(f.Doc.Text()), Build: buildConstraint(f)}
	for _, imp := range f.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err == nil {
			fi.Imports = append(fi.Imports, p)
		}
	}
	line := func(p token.Pos) int { return fset.Position(p).Line }

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			s := Symbol{Kind: "func", Name: d.Name.Name, Exported: d.Name.IsExported()}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := recvName(d.Recv.List[0].Type)
				s.Kind = "method"
				s.Name = recv + "." + d.Name.Name
				s.Exported = s.Exported && ast.IsExported(recv)
			}
			s.Doc = d.Doc.Text()
			s.Start, s.End = line(startPos(d.Doc, d.Pos())), line(d.End())
			sig := *d
			sig.Body, sig.Doc = nil, nil
			s.Signature = printNode(fset, &sig)
			fi.Symbols = append(fi.Symbols, s)

		case *ast.GenDecl:
			single := d.Lparen == token.NoPos
			for _, spec := range d.Specs {
				switch sp := spec.(type) {
				case *ast.TypeSpec:
					doc := sp.Doc
					from, to := sp.Pos(), sp.End()
					if single {
						if doc == nil {
							doc = d.Doc
						}
						from, to = d.Pos(), d.End()
					}
					s := Symbol{Kind: "type", Name: sp.Name.Name, Exported: sp.Name.IsExported(), Doc: doc.Text()}
					s.Start, s.End = line(startPos(doc, from)), line(to)
					stripComments(sp)
					s.Signature = "type " + printNode(fset, sp)
					fi.Symbols = append(fi.Symbols, s)
				case *ast.ValueSpec:
					doc := sp.Doc
					from, to := sp.Pos(), sp.End()
					if single {
						if doc == nil {
							doc = d.Doc
						}
						from, to = d.Pos(), d.End()
					}
					for _, n := range sp.Names {
						if n.Name == "_" {
							continue
						}
						s := Symbol{Kind: d.Tok.String(), Name: n.Name, Exported: n.IsExported(), Doc: doc.Text(), Signature: n.Name}
						s.Start, s.End = line(startPos(doc, from)), line(to)
						fi.Symbols = append(fi.Symbols, s)
					}
				}
			}
		}
	}
	return fi, nil
}

// buildConstraint returns the //go:build expression above the package
// clause in canonical form, or "" when there is none (or it is malformed).
func buildConstraint(f *ast.File) string {
	for _, g := range f.Comments {
		if g.Pos() >= f.Package {
			break
		}
		for _, c := range g.List {
			if !constraint.IsGoBuild(c.Text) {
				continue
			}
			if expr, err := constraint.Parse(c.Text); err == nil {
				return expr.String()
			}
		}
	}
	return ""
}

func startPos(doc *ast.CommentGroup, p token.Pos) token.Pos {
	if doc != nil {
		return doc.Pos()
	}
	return p
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.ParenExpr:
		return recvName(t.X)
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.IndexListExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return "?"
}

// stripComments removes doc/line comments so the printer emits a bare skeleton.
func stripComments(n ast.Node) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSpec:
			x.Doc, x.Comment = nil, nil
		case *ast.Field:
			x.Doc, x.Comment = nil, nil
		}
		return true
	})
}

func printNode(fset *token.FileSet, n any) string {
	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.TabIndent, Tabwidth: 4}
	if err := cfg.Fprint(&buf, fset, n); err != nil {
		return "<print error>"
	}
	// The printer aligns columns with runs of tabs; one space is enough for
	// a reader and costs fewer tokens.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for i, l := range lines {
		body := strings.TrimLeft(l, "\t")
		lines[i] = l[:len(l)-len(body)] + alignRe.ReplaceAllString(body, " ")
	}
	return strings.Join(lines, "\n")
}

var alignRe = regexp.MustCompile(`\t+`)
