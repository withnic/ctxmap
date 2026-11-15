package cmd

import (
	"flag"
	"strings"
)

// scanFlags are the file-selection flags shared by every subcommand.
type scanFlags struct {
	includeTests bool
	noGitignore  bool
	excludes     stringList
	keepSource   bool // not a flag: set by commands that print source
}

func addScanFlags(fs *flag.FlagSet) *scanFlags {
	sf := &scanFlags{}
	fs.BoolVar(&sf.includeTests, "include-tests", false, "include test files (*_test.go, *.spec.ts, *.test.ts, ...)")
	fs.BoolVar(&sf.noGitignore, "no-gitignore", false, "do not apply .gitignore rules (.ctxmapignore still applies)")
	fs.Var(&sf.excludes, "exclude", "exclude a .gitignore-style `pattern` relative to the scanned path (repeatable; \"!pattern\" re-includes)")
	return sf
}

// changeFlags restrict output to files changed in git.
type changeFlags struct {
	changed bool
	base    string
}

func addChangeFlags(fs *flag.FlagSet) *changeFlags {
	cf := &changeFlags{}
	fs.BoolVar(&cf.changed, "changed", false, "only files changed in git: uncommitted and untracked, plus commits since --base")
	fs.StringVar(&cf.base, "base", "", "with --changed: also include changes since the merge-base with `ref` (e.g. main)")
	return cf
}

// fileSet returns the changed files under root (keyed by pathKey), or nil
// when --changed is not set.
func (cf *changeFlags) fileSet(root string) (map[string]bool, error) {
	if !cf.changed {
		return nil, nil
	}
	return changedFiles(root, cf.base)
}

// filter keeps the files in set; a nil set keeps everything.
func filterFiles(files []*source, set map[string]bool) []*source {
	if set == nil {
		return files
	}
	var out []*source
	for _, f := range files {
		if set[pathKey(f.abs)] {
			out = append(out, f)
		}
	}
	return out
}

// stringList is a repeatable string flag.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}
