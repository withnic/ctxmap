package parser

import (
	"regexp"
	"sort"
	"strings"
)

// The TS/JS parser is intentionally lightweight: it blanks out comments and
// string contents (keeping byte offsets intact) so that brace matching works,
// then finds top-level declarations with anchored regular expressions.

var (
	tsDeclRe = regexp.MustCompile(`^(export\s+)?(default\s+)?(declare\s+)?(?:abstract\s+)?(?:async\s+)?` +
		`(function\s*\*?|class|interface|type|const\s+enum|enum|const|let|var|namespace|module)(?:\s+|\s*\*\s*)([A-Za-z_$][\w$]*)?`)
	tsDefaultRe   = regexp.MustCompile(`^export\s+default\s+`)
	tsCJSExportRe = regexp.MustCompile(`^(?:module\.)?exports\.([A-Za-z_$][\w$]*)\s*=`)

	tsImportRes = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\s*(?:import|export)\s[^;'"` + "`" + `]*?\bfrom\s*['"]([^'"\n]+)['"]`),
		regexp.MustCompile(`(?m)^\s*import\s*['"]([^'"\n]+)['"]`),
		regexp.MustCompile(`\brequire\s*\(\s*['"]([^'"\n]+)['"]\s*\)`),
		regexp.MustCompile(`\bimport\s*\(\s*['"]([^'"\n]+)['"]\s*\)`),
	}
)

type tsFile struct {
	src   string // original
	code  string // comments blanked
	mask  string // comments and string/template/regex contents blanked
	lines []int  // byte offset of each line start
	dts   bool
}

// ParseTS extracts exported declarations and imports from TS/JS source.
// Non-exported top-level declarations are returned too (Exported=false) so
// that slice can find them.
func ParseTS(path string, src []byte) (*FileInfo, error) {
	s := string(src)
	t := &tsFile{src: s, code: blank(s, false), mask: blank(s, true), dts: strings.HasSuffix(path, ".d.ts")}
	t.lines = []int{0}
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			t.lines = append(t.lines, i+1)
		}
	}
	fi := &FileInfo{Path: path, Lang: "ts", Imports: t.imports(), FileDoc: t.fileDoc()}
	fi.Symbols = t.decls()
	return fi, nil
}

func (t *tsFile) line(off int) int {
	return sort.Search(len(t.lines), func(i int) bool { return t.lines[i] > off })
}

func (t *tsFile) imports() []string {
	seen := map[string]bool{}
	var out []string
	for _, re := range tsImportRes {
		for _, m := range re.FindAllStringSubmatch(t.code, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

func (t *tsFile) decls() []Symbol {
	var syms []Symbol
	m, n := t.mask, len(t.mask)
	depth := 0
	for i := 0; i < n; {
		for i < n && (m[i] == ' ' || m[i] == '\t' || m[i] == '\r') {
			i++
		}
		if depth == 0 {
			if s, end, ok := t.tryDecl(i); ok {
				syms = append(syms, s)
				i = end
			}
		}
		for i < n && m[i] != '\n' {
			switch m[i] {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				if depth > 0 {
					depth--
				}
			}
			i++
		}
		i++
	}
	return syms
}

func (t *tsFile) tryDecl(i int) (Symbol, int, bool) {
	head := t.mask[i:min(len(t.mask), i+300)]
	var s Symbol
	var end int

	if mm := tsDeclRe.FindStringSubmatchIndex(head); mm != nil {
		group := func(k int) string {
			if mm[2*k] < 0 {
				return ""
			}
			return head[mm[2*k]:mm[2*k+1]]
		}
		kw := strings.Fields(group(4))[0]
		kw = strings.TrimSuffix(kw, "*")
		name := group(5)
		isDefault := group(2) != ""
		if name == "extends" || name == "implements" {
			name = ""
		}
		if name == "" {
			if !isDefault || (kw != "function" && kw != "class") {
				return s, 0, false
			}
			name = "default"
		}
		if kw == "module" && name == "exports" {
			return s, 0, false
		}
		s.Name = name
		s.Exported = group(1) != "" || (t.dts && group(3) != "")
		switch kw {
		case "function":
			s.Kind = "func"
			bodyOpen, stop := t.findBody(i, len(t.mask))
			if bodyOpen >= 0 {
				end = t.matchClose(bodyOpen) + 1
				s.Signature = t.compact(i, bodyOpen)
			} else {
				end = stop
				s.Signature = t.compact(i, end)
			}
		case "class":
			s.Kind = "class"
			open := strings.IndexByte(t.mask[i:], '{')
			if open < 0 {
				return s, 0, false
			}
			open += i
			close := t.matchClose(open)
			end = close + 1
			s.Signature = t.compact(i, open)
			s.Members = t.members(open+1, close)
		case "interface", "enum", "namespace", "module":
			s.Kind = kw
			if kw == "module" {
				s.Kind = "namespace"
			}
			open := strings.IndexByte(t.mask[i:], '{')
			if open < 0 {
				return s, 0, false
			}
			open += i
			end = t.matchClose(open) + 1
			if s.Kind == "namespace" {
				s.Signature = t.compact(i, open)
			} else {
				s.Signature = t.compact(i, end)
			}
		case "type":
			s.Kind = "type"
			end = t.stmtEnd(i, len(t.mask))
			s.Signature = t.compact(i, end)
		default: // const, let, var
			if strings.Contains(group(4), "enum") {
				s.Kind = "enum"
				open := strings.IndexByte(t.mask[i:], '{')
				if open < 0 {
					return s, 0, false
				}
				end = t.matchClose(open+i) + 1
				s.Signature = t.compact(i, end)
				break
			}
			s.Kind = kw
			end = t.stmtEnd(i, len(t.mask))
			s.Signature = t.valueSig(i, end)
		}
	} else if loc := tsDefaultRe.FindStringIndex(head); loc != nil {
		s = Symbol{Kind: "default", Name: "default", Exported: true}
		end = t.stmtEnd(i, len(t.mask))
		s.Signature = t.valueSig(i, end)
	} else if mm := tsCJSExportRe.FindStringSubmatch(head); mm != nil {
		s = Symbol{Kind: "var", Name: mm[1], Exported: true}
		end = t.stmtEnd(i, len(t.mask))
		s.Signature = t.valueSig(i, end)
	} else {
		return s, 0, false
	}

	doc, docStart := t.docBefore(i)
	s.Doc = doc
	s.Start = t.line(docStart)
	s.End = t.line(max(i, end-1))
	return s, end, true
}

// findBody returns the offset of the body '{' of a function-like declaration
// starting at i, or -1 and the end offset if the declaration has no body.
func (t *tsFile) findBody(i, limit int) (int, int) {
	m := t.mask
	depth := 0
	for j := i; j < limit; j++ {
		switch m[j] {
		case '{':
			if depth == 0 && t.isBodyBrace(i, j) {
				return j, 0
			}
			depth++
		case '(', '[':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return -1, j
			}
		case ';':
			if depth == 0 {
				return -1, j + 1
			}
		case '\n':
			if depth == 0 && j > i && !t.continues(j) && t.prevSig(j) == ')' && t.nextSig(j) != '{' {
				// e.g. an overload signature without ';' in a .d.ts
				return -1, j
			}
		}
	}
	return -1, limit
}

// isBodyBrace decides whether the '{' at j opens a body rather than an
// object type literal (e.g. a return type `: { a: string }`).
func (t *tsFile) isBodyBrace(start, j int) bool {
	k := j - 1
	for k >= start && isSpace(t.mask[k]) {
		k--
	}
	if k < start {
		return true
	}
	switch t.mask[k] {
	case ':', '|', '&', '<', ',', '=', '(', '?':
		return false
	}
	return true
}

// matchClose returns the offset of the bracket closing the one at open.
func (t *tsFile) matchClose(open int) int {
	m := t.mask
	depth := 0
	for j := open; j < len(m); j++ {
		switch m[j] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(m) - 1
}

// stmtEnd finds the end of a statement starting at i (';' or a newline that
// does not continue the expression).
func (t *tsFile) stmtEnd(i, limit int) int {
	m := t.mask
	depth := 0
	for j := i; j < limit; j++ {
		switch m[j] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return j
			}
		case ';':
			if depth == 0 {
				return j + 1
			}
		case '\n':
			if depth == 0 && !t.continues(j) {
				return j
			}
		}
	}
	return limit
}

func (t *tsFile) prevSig(j int) byte {
	for k := j - 1; k >= 0; k-- {
		if !isSpace(t.mask[k]) {
			return t.mask[k]
		}
	}
	return 0
}

func (t *tsFile) nextSig(j int) byte {
	for k := j + 1; k < len(t.mask); k++ {
		if !isSpace(t.mask[k]) {
			return t.mask[k]
		}
	}
	return 0
}

// continues reports whether the newline at j is inside an unfinished expression.
func (t *tsFile) continues(j int) bool {
	m := t.mask
	k := j - 1
	for k >= 0 && isSpace(m[k]) {
		k--
	}
	if k < 0 {
		return false
	}
	switch m[k] {
	case '=', ',', '|', '&', ':', '?', '(', '[', '{', '+', '-', '*', '/', '.', '<':
		return true
	case '>':
		if k > 0 && m[k-1] == '=' {
			return true
		}
	}
	l := j + 1
	for l < len(m) && isSpace(m[l]) {
		l++
	}
	if l >= len(m) {
		return false
	}
	switch m[l] {
	case '.', '|', '&', '?', ':':
		return true
	case '=':
		return l+1 < len(m) && m[l+1] == '>'
	}
	return false
}

var (
	tsModifiers = map[string]bool{"public": true, "private": true, "protected": true, "static": true,
		"readonly": true, "abstract": true, "override": true, "async": true, "declare": true, "accessor": true}
	identRe = regexp.MustCompile(`^#?[A-Za-z_$][\w$]*`)
)

// members extracts class members between from and to (exclusive).
func (t *tsFile) members(from, to int) []Symbol {
	m := t.mask
	var out []Symbol
	j := from
	for j < to {
		for j < to && (isSpace(m[j]) || m[j] == ';' || m[j] == ',') {
			j++
		}
		if j >= to {
			break
		}
		if m[j] == '@' { // decorator
			j++
			for j < to && (isIdent(m[j]) || m[j] == '.') {
				j++
			}
			if j < to && m[j] == '(' {
				j = t.matchClose(j) + 1
			}
			continue
		}
		start := j
		end, bodyOpen := to, -1
		depth := 0
	scan:
		for k := j; k < to; k++ {
			switch m[k] {
			case '{':
				if depth == 0 && t.isBodyBrace(start, k) {
					bodyOpen = k
					end = t.matchClose(k) + 1
					break scan
				}
				depth++
			case '(', '[':
				depth++
			case ')', ']', '}':
				depth--
			case ';':
				if depth == 0 {
					end = k + 1
					break scan
				}
			case '\n':
				if depth == 0 && !t.continues(k) {
					end = k
					break scan
				}
			}
		}
		if end <= start {
			end = start + 1
		}
		name, private := t.memberName(start, end)
		if name != "" {
			sig := ""
			if bodyOpen >= 0 {
				sig = t.compact(start, bodyOpen)
			} else {
				sig = truncateValue(t.compact(start, end))
			}
			doc, docStart := t.docBefore(start)
			out = append(out, Symbol{Kind: "member", Name: name, Signature: strings.TrimSuffix(sig, ";"),
				Doc: doc, Exported: !private, Start: t.line(docStart), End: t.line(max(start, end-1))})
		}
		j = end
	}
	return out
}

func (t *tsFile) memberName(start, end int) (string, bool) {
	s := t.mask[start:end]
	private := false
	for {
		s = strings.TrimLeft(s, " \t\r\n")
		w := identRe.FindString(s)
		if w == "" {
			if strings.HasPrefix(s, "*") { // generator method
				s = s[1:]
				continue
			}
			if strings.HasPrefix(s, "[") {
				return "[computed]", private
			}
			return "", private
		}
		rest := strings.TrimLeft(s[len(w):], " \t")
		isMod := tsModifiers[w] || ((w == "get" || w == "set") && rest != "" && (isIdent(rest[0]) || rest[0] == '#' || rest[0] == '['))
		if isMod && rest != "" && rest[0] != '(' && rest[0] != ':' && rest[0] != '=' && rest[0] != '?' && rest[0] != '<' {
			if w == "private" || w == "protected" {
				private = private || w == "private"
			}
			s = rest
			continue
		}
		return w, private || strings.HasPrefix(w, "#")
	}
}

// valueSig renders a const/let/var declaration without its (long) value.
func (t *tsFile) valueSig(i, end int) string {
	m := t.mask
	depth, eq := 0, -1
	for j := i; j < end && eq < 0; j++ {
		switch m[j] {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			if m[j] == '>' && j > 0 && m[j-1] == '=' {
				continue
			}
			depth--
		case '=':
			if depth == 0 && (j+1 >= end || (m[j+1] != '=' && m[j+1] != '>')) && (j == 0 || !strings.ContainsRune("=!<>", rune(m[j-1]))) {
				eq = j
			}
		}
	}
	if eq < 0 {
		return t.compact(i, end)
	}
	// arrow function at depth 0 => cut after "=>"
	depth = 0
	for j := eq + 1; j+1 < end; j++ {
		switch m[j] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '=':
			if depth == 0 && m[j+1] == '>' {
				return t.compact(i, j+2)
			}
		}
	}
	val := strings.TrimSpace(t.mask[eq+1 : end])
	if strings.HasPrefix(val, "function") || strings.HasPrefix(val, "async function") {
		if open, _ := t.findBody(eq+1, end); open >= 0 {
			return t.compact(i, open)
		}
	}
	return truncateValue(t.compact(i, end))
}

// truncateValue keeps short one-line values and abbreviates the rest.
func truncateValue(s string) string {
	s = strings.TrimSuffix(s, ";")
	if !strings.Contains(s, "\n") && len(s) <= 100 {
		return s
	}
	first := strings.SplitN(s, "\n", 2)[0]
	if len(first) > 80 {
		first = first[:80]
	}
	return strings.TrimRight(first, " ") + " …"
}

// compact returns code[from:to] with comments removed, trailing spaces and
// blank lines dropped.
func (t *tsFile) compact(from, to int) string {
	to = min(to, len(t.code))
	var out []string
	for _, l := range strings.Split(t.code[from:to], "\n") {
		l = strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return strings.TrimSpace(dedent(out))
}

func dedent(lines []string) string {
	if len(lines) <= 1 {
		return strings.Join(lines, "\n")
	}
	isClosing := func(s string) bool { return s != "" && strings.ContainsRune("})]", rune(s[0])) }
	// common indent of the body lines (closing brackets excluded)
	minIndent := -1
	for _, l := range lines[1:] {
		body := strings.TrimLeft(l, " \t")
		if n := len(l) - len(body); !isClosing(body) && (minIndent < 0 || n < minIndent) {
			minIndent = n
		}
	}
	// keep the first line as is; re-indent the body to one 2-space level
	// (relative nesting preserved) and closing brackets to column 0
	for i := 1; i < len(lines); i++ {
		body := strings.TrimLeft(lines[i], " \t")
		n := len(lines[i]) - len(body)
		if isClosing(body) && n <= max(minIndent, 0) {
			lines[i] = body
			continue
		}
		lines[i] = "  " + lines[i][min(n, max(minIndent, 0)):]
	}
	return strings.Join(lines, "\n")
}

// docBefore returns the JSDoc / line comments directly above offset i and
// the offset where they start.
func (t *tsFile) docBefore(i int) (string, int) {
	s := t.src
	lineStart := strings.LastIndexByte(s[:i], '\n') + 1
	if strings.TrimSpace(s[lineStart:i]) != "" || lineStart == 0 {
		return "", i
	}
	// previous line spans [ps, pe)
	prevLine := func(pos int) (int, int) {
		pe := pos - 1
		return strings.LastIndexByte(s[:pe], '\n') + 1, pe
	}
	ps, pe := prevLine(lineStart)
	if strings.HasSuffix(strings.TrimSpace(s[ps:pe]), "*/") {
		open := strings.LastIndex(s[:pe], "/*")
		if open < 0 || t.code[open] != ' ' { // must be a real comment
			return "", i
		}
		ls := strings.LastIndexByte(s[:open], '\n') + 1
		if strings.TrimSpace(s[ls:open]) != "" {
			return "", i
		}
		body := strings.TrimSpace(s[open+2 : pe])
		body = strings.TrimSuffix(strings.TrimPrefix(body, "*"), "*/")
		var lines []string
		for _, l := range strings.Split(body, "\n") {
			lines = append(lines, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "*")))
		}
		return strings.TrimSpace(strings.Join(lines, "\n")), ls
	}
	var lines []string
	start := i
	for pos := lineStart; pos > 0; {
		ps, pe := prevLine(pos)
		l := strings.TrimSpace(s[ps:pe])
		if !strings.HasPrefix(l, "//") {
			break
		}
		lines = append([]string{strings.TrimSpace(strings.TrimLeft(l, "/"))}, lines...)
		start, pos = ps, ps
	}
	if len(lines) == 0 {
		return "", i
	}
	return strings.Join(lines, "\n"), start
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func isIdent(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80
}

var regexKeywords = map[string]bool{"return": true, "typeof": true, "case": true, "do": true, "else": true,
	"in": true, "of": true, "new": true, "delete": true, "void": true, "throw": true, "yield": true, "await": true}

// blank replaces comments (and, if blankStrings, the contents of string,
// template and regex literals) with spaces, preserving offsets and newlines.
func blank(src string, blankStrings bool) string {
	b := []byte(src)
	out := []byte(src)
	n := len(b)
	clear := func(from, to int) {
		for k := from; k < to && k < n; k++ {
			if out[k] != '\n' {
				out[k] = ' '
			}
		}
	}
	var tmpl []int // brace depth per open ${ } expression
	inTemplate := false
	for i := 0; i < n; {
		c := b[i]
		if inTemplate {
			switch {
			case c == '\\':
				if blankStrings {
					clear(i, i+2)
				}
				i += 2
			case c == '`':
				inTemplate = false
				i++
			case c == '$' && i+1 < n && b[i+1] == '{':
				if blankStrings {
					clear(i, i+2)
				}
				tmpl = append(tmpl, 0)
				inTemplate = false
				i += 2
			default:
				if blankStrings {
					clear(i, i+1)
				}
				i++
			}
			continue
		}
		switch {
		case c == '/' && i+1 < n && b[i+1] == '/':
			j := i
			for j < n && b[j] != '\n' {
				j++
			}
			clear(i, j)
			i = j
		case c == '/' && i+1 < n && b[i+1] == '*':
			j := strings.Index(src[i+2:], "*/")
			end := n
			if j >= 0 {
				end = i + 2 + j + 2
			}
			clear(i, end)
			i = end
		case c == '\'' || c == '"':
			j := i + 1
			for j < n && b[j] != c && b[j] != '\n' {
				if b[j] == '\\' {
					j++
				}
				j++
			}
			if blankStrings {
				clear(i+1, j)
			}
			i = j + 1
		case c == '`':
			inTemplate = true
			i++
		case c == '{':
			if len(tmpl) > 0 {
				tmpl[len(tmpl)-1]++
			}
			i++
		case c == '}':
			if len(tmpl) > 0 {
				if tmpl[len(tmpl)-1] == 0 {
					tmpl = tmpl[:len(tmpl)-1]
					inTemplate = true
					if blankStrings {
						clear(i, i+1)
					}
					i++
					continue
				}
				tmpl[len(tmpl)-1]--
			}
			i++
		case c == '/' && isRegexStart(out, i):
			j := i + 1
			inClass := false
			for j < n && b[j] != '\n' {
				if b[j] == '\\' {
					j += 2
					continue
				}
				if b[j] == '/' && !inClass {
					break
				}
				switch b[j] {
				case '[':
					inClass = true
				case ']':
					inClass = false
				}
				j++
			}
			if j >= n || b[j] == '\n' { // not a regex after all
				i++
				continue
			}
			if blankStrings {
				clear(i+1, j)
			}
			i = j + 1
		default:
			i++
		}
	}
	return string(out)
}

func isRegexStart(out []byte, i int) bool {
	k := i - 1
	for k >= 0 && isSpace(out[k]) {
		k--
	}
	if k < 0 {
		return true
	}
	c := out[k]
	if isIdent(c) {
		e := k + 1
		for k >= 0 && isIdent(out[k]) {
			k--
		}
		return regexKeywords[string(out[k+1:e])]
	}
	return !strings.ContainsRune(")]}'\"`", rune(c))
}
