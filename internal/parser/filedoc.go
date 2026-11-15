package parser

import (
	"regexp"
	"strings"
)

var (
	// Comments at the top of a file that say nothing about its role:
	// license headers, generated-code markers and tool directives.
	skipCommentRe = regexp.MustCompile(`(?i)^(copyright\b|\(c\)|©|spdx-license-identifier|@license|@preserve|licensed under|all rights reserved|code generated|eslint-|eslint |@ts-|biome-ignore|prettier-ignore|istanbul ignore|c8 ignore|@jsx|@flow|<reference|global\s|jshint|#region|#endregion)`)
	// JSDoc file tags whose text is the description itself.
	fileTagRe = regexp.MustCompile(`(?i)^@(file|fileoverview|overview|module)\b[:\s]*`)
	// Directive prologue such as 'use client' or "use strict".
	directiveRe = regexp.MustCompile(`^(['"])use [A-Za-z ]+['"];?$`)
)

// utf8BOM is skipped at the start of a file.
const utf8BOM = "\xef\xbb\xbf"

// fileDoc returns the first comment at the top of the file that describes
// the file. Shebangs, directives, license headers and tool directives are
// skipped. A comment attached (no blank line) to a declaration is that
// declaration's doc and is not used; one attached to an import is.
func (t *tsFile) fileDoc() string {
	s := t.src
	i := 0
	if strings.HasPrefix(s, utf8BOM) {
		i = len(utf8BOM)
	}
	for {
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			return ""
		}
		line := strings.TrimSpace(s[i:lineEnd(s, i)])
		var text string
		var end int // offset just past the comment block
		switch {
		case strings.HasPrefix(line, "#!"), directiveRe.MatchString(line):
			i = lineEnd(s, i)
			continue
		case strings.HasPrefix(s[i:], "/*"):
			close := strings.Index(s[i+2:], "*/")
			if close < 0 {
				return ""
			}
			end = i + 2 + close + 2
			text = cleanComment(strings.Split(s[i+2:end-2], "\n"), "*!")
		case strings.HasPrefix(s[i:], "//"):
			var lines []string
			end = i
			for next := i; next < len(s); {
				l := strings.TrimSpace(s[next:lineEnd(s, next)])
				if !strings.HasPrefix(l, "//") {
					break
				}
				lines = append(lines, strings.TrimLeft(l, "/"))
				end = lineEnd(s, next) // end of this comment line, before its '\n'
				next = end + 1
			}
			text = cleanComment(lines, "")
		default:
			return "" // code reached without a description
		}
		i = end
		if text == "" || isBoilerplate(text) {
			continue
		}
		if t.attachedToDecl(end) {
			return "" // it documents the first declaration, not the file
		}
		return text
	}
}

// isBoilerplate reports license headers and tool directives.
func isBoilerplate(text string) bool {
	first := strings.SplitN(text, "\n", 2)[0]
	lower := strings.ToLower(text)
	return skipCommentRe.MatchString(first) ||
		(strings.Contains(lower, "copyright") && strings.Contains(lower, "licen"))
}

// attachedToDecl reports whether the code right after a comment ending at
// end starts on the next line (no blank line) and is a declaration.
func (t *tsFile) attachedToDecl(end int) bool {
	s := t.src
	j := end
	newlines := 0
	for j < len(s) && isSpace(s[j]) {
		if s[j] == '\n' {
			newlines++
		}
		j++
	}
	if j >= len(s) || newlines > 1 {
		return false
	}
	head := t.mask[j:min(len(t.mask), j+300)]
	if strings.HasPrefix(head, "import") || strings.HasPrefix(head, "/") {
		return false
	}
	return tsDeclRe.MatchString(head) || tsDefaultRe.MatchString(head) || tsCJSExportRe.MatchString(head)
}

// cleanComment strips comment markers (leading characters in trim, such as
// '*' of JSDoc lines) and JSDoc file tags, and drops leading/trailing blank
// lines.
func cleanComment(lines []string, trim string) string {
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if trim != "" {
			l = strings.TrimSpace(strings.TrimLeft(l, trim))
		}
		l = fileTagRe.ReplaceAllString(l, "")
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func lineEnd(s string, i int) int {
	if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(s)
}
