# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/) (see "Versioning" in the README).

## [Unreleased]

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

[Unreleased]: https://github.com/withnic/ctxmap/commits/main
