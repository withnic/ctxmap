package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// tsConfig is the subset of tsconfig.json / jsconfig.json that matters for
// resolving non-relative imports.
type tsConfig struct {
	baseURL  string // absolute, "" if unset
	pathsDir string // directory of the config that defines "paths"
	paths    []pathAlias
}

type pathAlias struct {
	prefix, suffix string // pattern split at '*'
	wildcard       bool
	targets        []string
}

// raw JSON shape (only the fields we use).
type tsConfigJSON struct {
	Extends         json.RawMessage `json:"extends"`
	CompilerOptions struct {
		BaseURL *string             `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
}

// maxExtendsDepth guards against extends cycles.
const maxExtendsDepth = 8

// tsConfigFor returns the config of the nearest tsconfig.json/jsconfig.json
// at or above dir (nil if none), cached per directory.
func (g *depGraph) tsConfigFor(dir string) *tsConfig {
	if c, ok := g.tsconfigs[dir]; ok {
		return c
	}
	var c *tsConfig
	found := false
	for _, name := range []string{"tsconfig.json", "jsconfig.json"} {
		if p := filepath.Join(dir, name); fileExists(p) {
			c, found = loadTSConfig(p, 0), true
			break
		}
	}
	if !found {
		if parent := filepath.Dir(dir); parent != dir {
			c = g.tsConfigFor(parent)
		}
	}
	g.tsconfigs[dir] = c
	return c
}

// loadTSConfig reads a config and the relative configs it extends. Package
// configs (e.g. "@tsconfig/node20") are not followed.
func loadTSConfig(path string, depth int) *tsConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw tsConfigJSON
	if err := json.Unmarshal(stripJSONC(data), &raw); err != nil {
		return nil
	}
	dir := filepath.Dir(path)
	c := &tsConfig{}
	if depth < maxExtendsDepth {
		for _, ext := range extendsList(raw.Extends) {
			if !strings.HasPrefix(ext, ".") {
				continue
			}
			p := filepath.Join(dir, filepath.FromSlash(ext))
			if !strings.HasSuffix(p, ".json") && !fileExists(p) {
				p += ".json"
			}
			if parent := loadTSConfig(p, depth+1); parent != nil {
				*c = *parent // later entries override earlier ones
			}
		}
	}
	if b := raw.CompilerOptions.BaseURL; b != nil {
		c.baseURL = filepath.Join(dir, filepath.FromSlash(*b))
	}
	if raw.CompilerOptions.Paths != nil {
		c.pathsDir = dir
		c.paths = nil
		for pattern, targets := range raw.CompilerOptions.Paths {
			a := pathAlias{targets: targets}
			if i := strings.IndexByte(pattern, '*'); i >= 0 {
				a.prefix, a.suffix, a.wildcard = pattern[:i], pattern[i+1:], true
			} else {
				a.prefix = pattern
			}
			c.paths = append(c.paths, a)
		}
		// TypeScript prefers the pattern with the longest prefix.
		sort.Slice(c.paths, func(i, j int) bool { return len(c.paths[i].prefix) > len(c.paths[j].prefix) })
	}
	return c
}

func extendsList(raw json.RawMessage) []string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	return nil
}

// candidates returns the absolute paths an import specifier may refer to.
// "paths" targets are relative to baseUrl when set, otherwise to the config
// that defines them (TypeScript 4.1+).
func (c *tsConfig) candidates(spec string) []string {
	base := c.baseURL
	if base == "" {
		base = c.pathsDir
	}
	var out []string
	for _, a := range c.paths {
		var star string
		switch {
		case a.wildcard && strings.HasPrefix(spec, a.prefix) && strings.HasSuffix(spec[len(a.prefix):], a.suffix):
			star = spec[len(a.prefix) : len(spec)-len(a.suffix)]
		case !a.wildcard && spec == a.prefix:
		default:
			continue
		}
		for _, t := range a.targets {
			out = append(out, filepath.Join(base, filepath.FromSlash(strings.Replace(t, "*", star, 1))))
		}
		break // only the best-matching pattern applies
	}
	if c.baseURL != "" {
		out = append(out, filepath.Join(c.baseURL, filepath.FromSlash(spec)))
	}
	return out
}

var trailingCommaRe = regexp.MustCompile(`,(\s*[}\]])`)

// stripJSONC removes // and /* */ comments (outside strings) and trailing
// commas, which tsconfig files commonly contain.
func stripJSONC(data []byte) []byte {
	var b strings.Builder
	s := string(data)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			end := min(j+1, len(s))
			b.WriteString(s[i:end])
			i = end - 1
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for i < len(s) && s[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				i = len(s)
			} else {
				i += end + 3
			}
		default:
			b.WriteByte(c)
		}
	}
	return []byte(trailingCommaRe.ReplaceAllString(b.String(), "$1"))
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
