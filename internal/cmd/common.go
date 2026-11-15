// Package cmd implements the ctxmap subcommands.
package cmd

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/withnic/ctxmap/internal/filter"
	"github.com/withnic/ctxmap/internal/parser"
)

// source is a parsed file together with its raw content.
type source struct {
	info *parser.FileInfo
	src  []byte
	abs  string
	disp string // path shown to the user
}

// scan is the result of walking and parsing one root.
type scan struct {
	keepSource bool // retain file contents (needed by slice)
	root       string
	base       string // directory output paths are relative to
	files      []*source
}

// newScan resolves where output paths are relative to: the working directory
// when root lies inside it (so paths can be fed straight back into Read
// tools), otherwise root itself (the header shows it), keeping lines short.
func newScan(root string) (*scan, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		abs = filepath.Dir(abs)
	}
	base := abs
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, abs); err == nil && !isOutside(rel) {
			base = wd
		}
	}
	return &scan{root: root, base: base}, nil
}

func isOutside(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// rel returns abs relative to the scan base, slash separated.
func (s *scan) rel(abs string) string {
	if r, err := filepath.Rel(s.base, abs); err == nil && !isOutside(r) {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(abs)
}

// label is the root as printed in headers.
func (s *scan) label() string {
	if abs, err := filepath.Abs(s.root); err == nil && s.rel(abs) == filepath.ToSlash(abs) {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(s.root)
}

// collect walks root and parses every matching file in parallel, keeping the
// walk order. Files that fail to read or parse are reported on stderr and
// skipped.
func collect(root string, sf *scanFlags) (*scan, error) {
	s, err := newScan(root)
	if err != nil {
		return nil, err
	}
	s.keepSource = sf.keepSource
	var paths []string
	opts := filter.Options{
		IncludeTests: sf.includeTests,
		Excludes:     sf.excludes,
		NoGitignore:  sf.noGitignore,
		OnError:      func(err error) { warn("", err) },
	}
	err = filter.Walk(root, opts, func(p string) error {
		paths = append(paths, p)
		return nil
	})
	if err != nil {
		return nil, err
	}

	parsed := make([]*source, len(paths))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				parsed[i] = s.parse(paths[i])
			}
		}()
	}
	for i := range paths {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	for _, f := range parsed {
		if f != nil {
			s.files = append(s.files, f)
		}
	}
	return s, nil
}

func (s *scan) parse(p string) *source {
	disp := s.rel(p)
	src, err := os.ReadFile(p)
	if err != nil {
		warn(disp, err)
		return nil
	}
	// parse under the display path: parser errors already start with it
	fi, err := parser.ParseFile(disp, src)
	if err != nil {
		warn("", err)
		return nil
	}
	f := &source{info: fi, abs: p, disp: disp}
	if s.keepSource {
		f.src = src // only slice prints source; dropping it halves peak memory
	}
	return f
}

func warn(path string, err error) {
	if path != "" {
		fmt.Fprintf(os.Stderr, "ctxmap: warning: %s: %v\n", path, err)
		return
	}
	fmt.Fprintf(os.Stderr, "ctxmap: warning: %v\n", err)
}

// parseArgs parses flags that may appear before or after positional args.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func newFlagSet(name, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: ctxmap %s\n", usage)
		fs.PrintDefaults()
	}
	return fs
}

// Exit codes shared by all subcommands.
const (
	exitOK    = 0
	exitFail  = 1 // runtime failure or nothing found
	exitUsage = 2 // bad flags / arguments
)

func flagExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	return exitUsage
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "ctxmap:", err)
	return exitFail
}

// writeOut runs render against a buffered stdout and reports write errors
// (e.g. a closed pipe), which a deferred Flush would silently drop.
func writeOut(render func(w io.Writer) int) int {
	w := bufio.NewWriter(os.Stdout)
	code := render(w)
	if err := w.Flush(); err != nil {
		return fail(err)
	}
	return code
}
