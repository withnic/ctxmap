# ctxmap

English | [日本語](README.ja.md)

A CLI that keeps AI agents from burning tokens by reading whole files over and over while exploring a codebase.
It prints only the **skeleton of the code (signatures, type definitions, doc-comment summaries)** and its **local dependencies**, as compact Markdown on stdout.

- Languages: Go (exact parsing with `go/ast`), TypeScript / JavaScript (`.ts` `.tsx` `.js` `.jsx`, lightweight parser)
- No external dependencies (standard library only); a single binary, no CGO
- On this repository, `map` output is about 20% of the size of the (non-test) source

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

A line starting with `> ` right under a file heading says what the file is for: the Go package comment (so a `doc.go` that holds nothing else now shows up too) or the leading comment of a TS/JS file. License headers and tool directives (`Copyright ...`, `SPDX-License-Identifier`, `eslint-disable`, `@ts-nocheck`, ...) are skipped, and a comment attached directly to the first declaration stays that declaration's doc.

```text
$ ctxmap map internal/parser
# map internal/parser (4 files)

## internal/parser/go_parser.go [parser]
// ParseGo parses a Go file with go/parser and returns its skeleton.
func ParseGo(path string, src []byte) (*FileInfo, error) #L14-88
func recvName(e ast.Expr) string #L97-111

## internal/parser/parser.go [parser]
> Package parser extracts skeletons (signatures, type definitions, doc comments) and imports from Go and TypeScript/JavaScript sources.
// Symbol is one top-level declaration (or a class member).
type Symbol struct { #L13-23
	Kind string
	Name string
	...
}
```

For a first look at a large repository, `--summary` prints one line per directory, followed by its description: a Go `doc.go` first, then any package comment; for TS/JS, the leading comment of an `index` file.

```text
$ ctxmap map --summary
# map . --summary (14 files, 145 symbols)
. 1 files, 3 symbols
  > Command ctxmap prints token-efficient code skeletons and dependency maps for AI agents exploring a codebase.
internal/cmd/ 8 files, 80 symbols: Deps, Map, Slice
  > Package cmd implements the ctxmap subcommands.
internal/filter/ 1 files, 22 symbols: DefaultIgnoreDirs, DefaultIgnoreFiles, SourceExts, IsTestFile, IsSourceFile, IsIgnoredFile, IgnoreFileName, Options, …
  > Package filter decides which files ctxmap scans: default exclusions, test-file detection, and simplified .gitignore / .ctxmapignore matching.
internal/parser/ 4 files, 40 symbols: ParseGo, Symbol, FileInfo, ErrUnsupported, ParseFile, Summary, Intro, ParseTS
  > Package parser extracts skeletons (signatures, type definitions, doc comments) and imports from Go and TypeScript/JavaScript sources.
```

| Flag | Description |
|---|---|
| `--summary` | One line per directory: file/symbol counts, the main exported names and the package description |
| `--exported` | Go: show exported symbols only (TS/JS always shows exported declarations only) |
| `--full-doc` | Print whole doc comments (file/package descriptions: up to the first heading, at most 15 lines) |
| `--no-doc` | Omit doc comments, including file/package descriptions |
| `--no-lines` | Omit the `#Lstart-end` line references |
| `--max-lines N` | Stop after about N lines (default `500`, `0` = unlimited); see below |
| `--changed` | Only files changed in git (staged, unstaged, untracked) |
| `--base <ref>` | With `--changed`: also include everything changed since the merge-base with `ref` (e.g. `main`), which is what a PR contains |

- Go `const` / `var` declarations are listed by exported name only, one line per kind (e.g. `const ModeA, ModeB`).
- TS/JS classes show public member signatures only (`private` and `#private` members are left out).

### `ctxmap deps [path]` — local dependencies

Lists imports within the repository; external packages are excluded. Go dependencies are shown per package directory, TS/JS dependencies per resolved file.

```text
$ ctxmap deps
# deps . (local only; 4/14 files have local deps)
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

TS/JS import resolution:

- Relative imports: `./foo`, `./foo.js` (when the file is actually `foo.ts`), `./dir` (`dir/index.ts`)
- Aliases from the nearest `tsconfig.json` / `jsconfig.json`: `compilerOptions.paths` (e.g. `@/*`) and `baseUrl`, including configs pulled in with a relative `extends`. Comments and trailing commas in the config are fine.
- Anything that does not resolve to a file is treated as an external package.

### `ctxmap slice <symbol>... [path]` — full definitions

Prints the complete definition of functions, methods, types, classes or members, including body and doc comment, with the file path and line range.
Several symbols can be requested at once, separated by spaces or commas.

````text
$ ctxmap slice Summary withLineRef
## internal/parser/parser.go:52-88 (func Summary)
```go
// Summary returns the first sentence of a doc comment's first paragraph,
func Summary(doc string) string {
	...
}
```

## internal/cmd/map.go:212-223 (func withLineRef)
```go
...
```
````

- Symbols are given as `Name`, `Recv.Method` (Go) or `Class.member` (TS/JS).
- Without an exact match, the search falls back to a case-insensitive match. Missing symbols are reported on stderr with similar candidates; the ones that were found are still printed, and the exit code is 1 if any symbol was missing.
- The last argument is the search path when it exists on disk (`ctxmap slice Map cmd`). If a symbol could be mistaken for a path, pass the path with `--path` instead.

### Common behavior

- Flags may come before or after positional arguments (`ctxmap map src --exported`).
- Paths under the current directory are printed relative to it, so they can be passed straight to a Read tool.
- Files that fail to parse are reported on stderr and skipped; processing continues.
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
| `--include-tests` | Include test files (`*_test.go` `*.spec.*` `*.test.*`) |
| `--exclude <pattern>` | Extra `.gitignore`-style pattern, relative to the scanned path. Repeatable; `!pattern` re-includes |
| `--no-gitignore` | Do not apply `.gitignore` (`.ctxmapignore` still applies) |

Excluded by default:

| Kind | Patterns |
|---|---|
| Directories | `.git` `node_modules` `vendor` `dist` `build` `.next` `.claude` |
| Files | `go.sum` `package-lock.json` `yarn.lock` `pnpm-lock.yaml` `*.min.js` `*.pb.go` |
| Tests | `*_test.go` `*.spec.*` `*.test.*` (included with `--include-tests`) |

### `.ctxmapignore`

Put a `.ctxmapignore` file (same syntax as `.gitignore`) in any directory to hide things that are tracked by git but not worth showing to an agent: generated code, `testdata/`, `examples/`, mocks, and so on. Commit it so that the whole team and CI get the same view.

```gitignore
# .ctxmapignore
mocks/
testdata/
*.gen.go
# re-include a directory excluded by default
!build/
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
internal/cmd/    map / deps / slice, shared flags, git changes, tsconfig resolution
internal/parser/ Go (go/ast) and TS/JS (lightweight) parsers
internal/filter/ default exclusions, .gitignore / .ctxmapignore / --exclude matching
doc/             requirements, design, guidelines, CLAUDE.md template (Japanese)
skills/ctxmap/   Claude Code skill (`make install-skill`)
```

Design details (in Japanese): [doc/01_system_overview.md](doc/01_system_overview.md), [doc/02_architecture.md](doc/02_architecture.md), [doc/03_guidelines.md](doc/03_guidelines.md).

## Known limitations

- TS/JS parsing is not a full AST parse. Text inside JSX containing characters such as `'` can occasionally confuse the parser for that line.
- tsconfig `extends` is followed only for relative paths; configs from packages (e.g. `@tsconfig/node20`) are not read.
- `--changed` needs `git` on `PATH` and a repository with at least one commit.

## Versioning

ctxmap follows [Semantic Versioning](https://semver.org/). Its compatibility surface is the CLI: command names, flags, exit codes and the output format that agents read (there is no importable Go API; all packages are under `internal/`).

- **Before v1.0.0**, a minor release (`v0.x.0`) may change flags or the output format; such changes are listed in [CHANGELOG.md](CHANGELOG.md). Patch releases (`v0.x.y`) only fix bugs.
- **From v1.0.0**, breaking changes require a new major version.

Pin a version when reproducibility matters, e.g. `go install github.com/withnic/ctxmap@v0.1.0`.

## License

[MIT](LICENSE)
