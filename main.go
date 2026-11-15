// Command ctxmap prints token-efficient code skeletons and dependency maps
// for AI agents exploring a codebase.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/withnic/ctxmap/internal/cmd"
)

// version is set at build time: go build -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `ctxmap - token-efficient code maps for AI agents

usage:
  ctxmap map   [path] [--summary] [--exported] [--full-doc|--no-doc] [--no-lines] [--changed [--base ref]] [--max-lines N]
  ctxmap deps  [path] [--target <file|dir|package> [--depth N]] [--changed [--base ref]] [--max-lines N]
  ctxmap slice <symbol>[,<symbol>...] [<symbol>...] [path] [--path p]
  ctxmap version

commands:
  map    signatures, type definitions and doc comments (bodies omitted),
         each with a #Lstart-end line reference; --summary: one line per dir
  deps   local import graph (Go packages, TS/JS files incl. tsconfig paths);
         --target narrows to one file/package, --depth follows it further
  slice  full source of symbols (Name, Recv.Method, Class.member)

file selection (all commands):
  --include-tests     include *_test.go, *.spec.ts, *.test.ts, ...
  --exclude <pattern> .gitignore-style pattern, repeatable ("!p" re-includes)
  --no-gitignore      ignore .gitignore (.ctxmapignore still applies)
  .ctxmapignore       per-directory ignore file, .gitignore syntax

Run "ctxmap <command> -h" for all flags.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	rest := args[1:]
	switch args[0] {
	case "map":
		return cmd.Map(rest)
	case "deps":
		return cmd.Deps(rest)
	case "slice":
		return cmd.Slice(rest)
	case "version", "--version":
		fmt.Println("ctxmap", buildVersion())
		return 0
	case "-h", "--help", "help":
		fmt.Print(usage)
		return 0
	}
	fmt.Fprintf(os.Stderr, "ctxmap: unknown command %q\n\n%s", args[0], usage)
	return 2
}

// buildVersion prefers the -ldflags value, then the module version recorded
// by `go install module@version`, then the VCS revision.
func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(devel)"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 12 {
			return "(devel) " + s.Value[:12]
		}
	}
	return "(devel)"
}
