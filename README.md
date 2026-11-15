# ctxmap

English | [日本語](README.ja.md)

A CLI that keeps AI agents from burning tokens by reading whole files over and over while exploring a **Go** codebase.
It prints only the **skeleton of the code (signatures, type definitions, doc-comment summaries)** and its **local dependencies**, as compact Markdown on stdout.

- Go only, parsed exactly with `go/ast` (TypeScript/JavaScript support was removed in v0.2.0; see [Other languages](#other-languages))
- No external dependencies (standard library only); a single binary, no CGO
- On this repository, `map` output is about 25% of the size of the (non-test) source

## Installation

Requires Go 1.22 or later.

```sh
go install github.com/withnic/ctxmap@latest
# or from source
make install
```

Make sure `$(go env GOPATH)/bin` (`%USERPROFILE%\go\bin` on Windows) is on your `PATH`.

## Usage

### `ctxmap map [path]` — code skeleton

Prints declarations with function bodies omitted, plus the first sentence of each doc comment.
Each declaration ends with a `#Lstart-end` line reference (doc comment included), so a reader can fetch exactly that range.

A line starting with `> ` right under a file heading says what the file is for: the package comment (so a `doc.go` that holds nothing else shows up too). A license header separated from the package clause by a blank line is not part of it.

```text
$ ctxmap map internal/parser
# map internal/parser (2 files)

## internal/parser/go_parser.go [parser]
// ParseGo parses a Go file with go/parser and returns its skeleton.
func ParseGo(path string, src []byte) (*FileInfo, error) #L14-88
func recvName(e ast.Expr) string #L97-111

## internal/parser/parser.go [parser]
> Package parser extracts skeletons (signatures, type definitions, doc comments) and imports from Go source files.
// Symbol is one top-level declaration.
type Symbol struct { #L13-22
	Kind string
	Name string
	...
}
```

For a first look at a large repository, `--summary` prints one line per directory, followed by the package description (`doc.go` first, then any package comment).

```text
$ ctxmap map --summary
# map . --summary (11 files, 103 symbols)
. 1 files, 3 symbols
  > Command ctxmap prints token-efficient code skeletons and dependency maps for AI agents exploring a codebase.
internal/cmd/ 7 files, 68 symbols: Deps, Map, Slice
  > Package cmd implements the ctxmap subcommands.
internal/filter/ 1 files, 21 symbols: DefaultIgnoreDirs, DefaultIgnoreFiles, IsTestFile, IsSourceFile, IsIgnoredFile, IgnoreFileName, Options, Walk
  > Package filter decides which files ctxmap scans: default exclusions, test-file detection, and simplified .gitignore / .ctxmapignore matching.
internal/parser/ 2 files, 11 symbols: ParseGo, Symbol, FileInfo, ErrUnsupported, ParseFile, Summary, Intro
  > Package parser extracts skeletons (signatures, type definitions, doc comments) and imports from Go source files.
```

| Flag | Description |
|---|---|
| `--summary` | One line per directory: file/symbol counts, the main exported names and the package description |
| `--exported` | Show exported symbols only |
| `--full-doc` | Print whole doc comments (file/package descriptions: up to the first heading, at most 15 lines) |
| `--no-doc` | Omit doc comments, including file/package descriptions |
| `--no-lines` | Omit the `#Lstart-end` line references |
| `--max-lines N` | Stop after about N lines (default `500`, `0` = unlimited); see below |
| `--changed` | Only files changed in git (staged, unstaged, untracked) |
| `--base <ref>` | With `--changed`: also include everything changed since the merge-base with `ref` (e.g. `main`), which is what a PR contains |

- Unexported functions and types are shown too (they are part of the skeleton); `--exported` hides them.
- `const` / `var` declarations are listed by exported name only, one line per kind (e.g. `const ModeA, ModeB`).

### `ctxmap deps [path]` — local dependencies

Lists the imports between packages of the module (resolved through the nearest `go.mod`), one line per file, as package directories. Standard library and third-party imports are left out.

```text
$ ctxmap deps
# deps . (local only; 4/11 files have local deps)
internal/cmd/common.go -> internal/filter, internal/parser
internal/cmd/map.go -> internal/parser
main.go -> internal/cmd
```

With `--target`, the output narrows to what the target **depends on** and what it is **referenced by**. The target can be a file path, a directory, a Go import path, a package name, or a file name without its extension.
`--depth N` follows the graph N levels in both directions (`0` = unlimited); each entry is prefixed with its distance.

```text
$ ctxmap deps --target filter --depth 0
# deps --target filter --depth 0
target: internal/filter/ignore.go
depends on (0):
referenced by (2):
- [1] internal/cmd/common.go
- [2] main.go
```

| Flag | Description |
|---|---|
| `--target <x>` | Narrow to one file / directory / package |
| `--depth N` | With `--target`: levels to follow (default `1`, `0` = unlimited) |
| `--changed`, `--base <ref>` | Without `--target`: list only changed files (same meaning as in `map`) |
| `--max-lines N` | Without `--target`: stop the listing after about N lines (default `500`, `0` = unlimited) |

### `ctxmap slice <symbol>... [path]` — full definitions

Prints the complete definition of functions, methods, types, consts or vars, including body and doc comment, with the file path and line range.
Several symbols can be requested at once, separated by spaces or commas.

````text
$ ctxmap slice Summary withLineRef
## internal/parser/parser.go:47-83 (func Summary)
```go
// Summary returns the first sentence of a doc comment's first paragraph,
func Summary(doc string) string {
	...
}
```

## internal/cmd/map.go:187-198 (func withLineRef)
```go
...
```
````

- Symbols are given as `Name` or `Recv.Method`.
- Slicing a type also lists its methods from every file of the package (signature and location only):
  ```text
  methods on depGraph (10):
  - internal/cmd/deps.go:106-125 func (g *depGraph) writeAll(w io.Writer, files []*source, changedOnly bool, maxLines int)
  ...
  ```
- Without an exact match, the search falls back to a case-insensitive match. Symbols that are still not found are listed at the end of stdout with suggestions, e.g. `> [not found] "ParseGoo" (did you mean: ParseGo (internal/parser/go_parser.go)?)`. The exit code is 0 when at least one symbol was found and 1 when none was.
- The last argument is the search path only when it clearly is a path and exists: `.`, `..`, anything containing `/` or `\`, or a `.go` file (`ctxmap slice Map ./internal/cmd`). A single word such as `cmd` is always a symbol; use `--path cmd` for a directory with a one-word name.

### Common behavior

- Flags may come before or after positional arguments (`ctxmap map src --exported`).
- Paths under the current directory are printed relative to it, so they can be passed straight to a Read tool.
- Files that fail to parse are reported on stderr and skipped; processing continues. Files over 10 MB (almost always generated code) are skipped the same way.
- **Output limit**: `map`, `map --summary` and the `deps` listing stop after about 500 lines by default so a large repository cannot flood an agent's context. Output is cut only between whole files (directories for `--summary`), the header still shows the full counts, and the last line says what was left out and how to narrow the request:

  ```text
  ... (truncated at --max-lines 500: 4808 more files, 153844 lines not shown; narrow with 'ctxmap map --summary' or a subdirectory path, or pass --max-lines 0)
  ```

  Use `--max-lines N` to change the limit or `--max-lines 0` to turn it off. `slice` is not limited, since it prints only the symbols asked for.
- Exit codes: `0` success / `1` runtime error or nothing found / `2` usage error
- `ctxmap version` prints the version.

## Choosing files

All commands share these flags:

| Flag | Description |
|---|---|
| `--include-tests` | Include test files (`*_test.go`) |
| `--exclude <pattern>` | Extra `.gitignore`-style pattern, relative to the scanned path. Repeatable; `!pattern` re-includes |
| `--no-gitignore` | Do not apply `.gitignore` (`.ctxmapignore` still applies) |

Excluded by default:

| Kind | Patterns |
|---|---|
| Directories | `.git` `vendor` `testdata` `node_modules` `.claude` |
| Files | `*.pb.go` (generated protobuf code) |
| Tests | `*_test.go` (included with `--include-tests`) |

### `.ctxmapignore`

Put a `.ctxmapignore` file (same syntax as `.gitignore`) in any directory to hide things that are tracked by git but not worth showing to an agent: generated code, `testdata/`, `examples/`, mocks, and so on. Commit it so that the whole team and CI get the same view.

```gitignore
# .ctxmapignore
mocks/
examples/
*.gen.go
# re-include a directory excluded by default
!testdata/
```

As in `.gitignore`, a comment must be on its own line; `# ...` after a pattern becomes part of the pattern.

Rules are applied in this order; later rules win:

1. default exclusions
2. `.gitignore`
3. `.ctxmapignore`
4. `--exclude`

`.gitignore` can only add exclusions; a `!` rule there does not bring back a default exclusion such as `vendor/`. `.ctxmapignore` and `--exclude` can.
Ignore files are read from every directory under the scanned path and from parent directories up to the repository root.
The matcher is a simplified implementation that supports `*`, `**`, `?`, `[...]`, negation with `!`, a trailing `/` (directories only) and a leading `/` (anchored).

## Using it with AI agents

### Claude Code skill (recommended)

[skills/ctxmap/SKILL.md](skills/ctxmap/SKILL.md) is a [Claude Code skill](https://docs.claude.com/en/docs/claude-code/skills) (instructions in Japanese) that teaches the agent the flow below, how to read the output (`#L` ranges, `> ` descriptions, the truncation notice) and the commands. Installed once as a user skill, it works in every project, and it pre-approves the read-only `ctxmap` commands so they run without a permission prompt.

```sh
# from a clone of this repository
make install-skill
# or without cloning
mkdir -p ~/.claude/skills/ctxmap
curl -fsSL https://raw.githubusercontent.com/withnic/ctxmap/main/skills/ctxmap/SKILL.md -o ~/.claude/skills/ctxmap/SKILL.md
```

Start a new Claude Code session afterwards. A skill is loaded when the agent decides it is relevant, so in projects where it should always be used, add a line to that project's `CLAUDE.md`:

```markdown
## Code exploration
Before reading source files, use the ctxmap skill (project structure, finding definitions, impact of a change, reviewing changes).
```

### Without the skill

[doc/ctxmap_rules_template.md](doc/ctxmap_rules_template.md) is the same set of rules (in Japanese) to paste into a project's `CLAUDE.md` instead.

### The basic flow

1. Get the big picture with `ctxmap map` (`ctxmap map --summary` first on large repositories)
2. Check the impact of a change with `ctxmap deps --target` (add `--depth` for indirect impact)
3. Read only the definitions you need with `ctxmap slice` (several at once), or Read just the `#L` range from `map`
4. Read whole files only as a last resort

For reviews, `ctxmap map --changed --base main` shows the skeleton of just the files a branch touches.

## Development

Requires GNU make and a POSIX shell (on Windows, run from Git Bash). golangci-lint and govulncheck are pinned in the Makefile and installed into `./bin/tools` on first use.

```sh
make help     # list targets
make check    # fmt-check + vet + lint + test (run before committing)
make ci       # all checks incl. tidy-check + build + smoke test
make cover    # coverage
make vuln     # known-vulnerability scan
make install-skill  # copy the Claude Code skill to ~/.claude/skills
make build VERSION=v0.1.0
```

The lint configuration lives in [.golangci.yml](.golangci.yml): the standard set plus revive, gocritic, errorlint, misspell, unconvert, unparam and nolintlint, with gofmt and goimports as formatters.

### Layout

```text
main.go          entry point and subcommand dispatch
internal/cmd/    map / deps / slice, shared flags, git changes
internal/parser/ Go parser (go/ast)
internal/filter/ default exclusions, .gitignore / .ctxmapignore / --exclude matching
doc/             requirements, design, guidelines, CLAUDE.md template (Japanese)
skills/ctxmap/   Claude Code skill (`make install-skill`)
```

Design details (in Japanese): [doc/01_system_overview.md](doc/01_system_overview.md), [doc/02_architecture.md](doc/02_architecture.md), [doc/03_guidelines.md](doc/03_guidelines.md).

## Known limitations

- Go only. Build constraints are not evaluated: files for every platform are listed, and each file heading shows its `//go:build` constraint, e.g. `## os/dirent_wasip1.go [os] (go:build wasip1)`.
- `--changed` needs `git` on `PATH` and a repository with at least one commit.

## Other languages

ctxmap supported TypeScript/JavaScript with a lightweight parser up to v0.1.0. It was removed in v0.2.0: guessing at TS syntax silently missed declarations (separate `export { ... }`, barrel files, ...), which is worse for an agent than not using the tool. For TypeScript, the compiler gives an exact skeleton instead:

```sh
npx tsc --declaration --emitDeclarationOnly --outDir /tmp/skeleton
```

v0.1.0 is still installable (`go install github.com/withnic/ctxmap@v0.1.0`) if you need its TS/JS support.

## Versioning

ctxmap follows [Semantic Versioning](https://semver.org/). Its compatibility surface is the CLI: command names, flags, exit codes and the output format that agents read (there is no importable Go API; all packages are under `internal/`).

- **Before v1.0.0**, a minor release (`v0.x.0`) may change flags or the output format; such changes are listed in [CHANGELOG.md](CHANGELOG.md). Patch releases (`v0.x.y`) only fix bugs.
- **From v1.0.0**, breaking changes require a new major version.

Pin a version when reproducibility matters, e.g. `go install github.com/withnic/ctxmap@v0.1.0`.

## License

[MIT](LICENSE)
