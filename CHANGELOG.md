# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/) (see "Versioning" in the README).

## [Unreleased]

ctxmap is now a Go-only tool.

### Removed
- TypeScript/JavaScript support: the lightweight TS/JS parser, `tsconfig.json` /
  `jsconfig.json` path resolution, TS/JS file descriptions and `Class.member`
  symbols. Guessing at TS syntax silently missed declarations (for example a
  separate `export { ... }` or barrel files), which misleads agents. For
  TypeScript, `tsc --declaration --emitDeclarationOnly` gives an exact skeleton;
  v0.1.0 remains installable for the old behavior.

### Changed
- Default exclusions are now `.git`, `vendor`, `testdata`, `node_modules` and
  `.claude` directories and `*.pb.go` files. `dist`, `build` and `.next` (JS build
  output) are no longer excluded, since `build` or `dist` can be ordinary Go
  packages (e.g. `cmd/dist` in the Go source tree).
- `deps` resolves imports against the nearest `go.mod` only.
- `slice` exits with 0 when at least one symbol was found (1 only when none
  was); symbols that were not found are listed at the end of stdout as
  `> [not found] "Name" (did you mean: ...?)` instead of on stderr.
- `slice` treats its last argument as the search path only when it clearly is
  one (`.`, `..`, contains `/` or `\`, or ends in `.go`) and exists, so a symbol
  named like a directory (`slice Config config`) is still searched; use `--path`
  for a one-word directory name.

### Added
- `slice <Type>` lists the type's methods from every file of the package
  (signature and location) after the definition.
- `map` file headings show the file's `//go:build` constraint, e.g.
  `## dirent_wasip1.go [os] (go:build wasip1)`, so platform-specific variants of
  the same code can be told apart.
- Files over 10 MB (almost always generated code) are skipped with a warning.

## [0.1.0] - 2026-11-16

First public release.

### Added
- `ctxmap map [path]`: compact Markdown skeleton of the code — signatures, type
  definitions and doc-comment summaries, with function bodies omitted.
  - `#Lstart-end` line reference on every declaration (`--no-lines` to omit)
  - `--summary`: one line per directory with file/symbol counts and main exported names
  - `--exported`, `--full-doc`, `--no-doc`
  - file role under each file heading (`> ...`): the Go package comment (so a `doc.go`
    is shown too) or the leading TS/JS comment, skipping license headers and tool directives
  - `--summary` adds each directory's description (`doc.go` first, then any package
    comment; TS/JS: the leading comment of an `index` file)
- `ctxmap deps [path]`: local import graph (Go per package, TS/JS per resolved file).
  - `--target <file|dir|package>` narrows to what the target depends on and what references it
  - `--depth N` follows the graph further (`0` = unlimited)
  - TS/JS: relative imports, `tsconfig.json` / `jsconfig.json` `paths` and `baseUrl`
    (relative `extends`, JSONC comments and trailing commas supported)
- `ctxmap slice <symbol>... [path]`: full source of one or more symbols
  (`Name`, `Recv.Method`, `Class.member`) with file path and line range; `--path`.
- `ctxmap version`.
- Claude Code skill (`skills/ctxmap/SKILL.md`, installed with `make install-skill`) that
  teaches agents to explore code with ctxmap before reading files.
- Output limit for `map`, `map --summary` and the `deps` listing: `--max-lines N`
  (default `500`, `0` = unlimited). Output is cut between whole files/directories, the
  header keeps the full counts, and a trailing notice says how much was left out and
  how to narrow the request.
- File selection shared by all commands: default exclusions, `.gitignore`,
  per-directory `.ctxmapignore`, `--exclude <pattern>`, `--no-gitignore`, `--include-tests`.
- `--changed [--base <ref>]` for `map` and `deps`: only files changed in git.
- Go parsing with `go/ast`; lightweight TS/JS parser for `.ts` `.tsx` `.js` `.jsx`.
- No external dependencies; single static binary (no CGO). Supports Go 1.22+ on
  Linux, macOS and Windows.

### Security
- `--base` values that start with `-` are rejected so they cannot reach `git` as options.

[Unreleased]: https://github.com/withnic/ctxmap/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/withnic/ctxmap/releases/tag/v0.1.0
