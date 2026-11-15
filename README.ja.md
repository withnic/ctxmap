# ctxmap

[English](README.md) | 日本語

AIエージェントがコードベースを探索するときに、ファイル全体を何度も Read してトークンを浪費しないようにするCLIです。
コードの **骨格（シグネチャ・型定義・Docコメント要約）** と **ローカル依存関係** だけを、コンパクトなMarkdownで標準出力に返します。

- 対応言語: Go（`go/ast` による正確な解析）、TypeScript / JavaScript（`.ts` `.tsx` `.js` `.jsx`、軽量パーサー）
- 外部依存なし（標準ライブラリのみ）、CGO不要の単一バイナリ
- このリポジトリ自身では、`map` の出力がソース（テストを除く）の約20%に収まります

## インストール

Go 1.22 以上が必要です。

```sh
go install github.com/withnic/ctxmap@latest
# またはソースから
make install
```

`$(go env GOPATH)/bin`（Windowsでは `%USERPROFILE%\go\bin`）が `PATH` に入っていることを確認してください。

## 使い方

### `ctxmap map [path]` — コードの骨格

関数本体は省略し、宣言と、Docコメントの最初の1文だけを出力します。
各宣言の末尾には `#L開始-終了` の行番号（Docコメントを含む範囲）が付くので、その範囲だけを Read できます。

ファイル見出しの直下にある `> ` で始まる行は、そのファイルの役割の説明です。Go では package コメント（ほかに何もない `doc.go` も表示されます）、TS/JS ではファイル先頭のコメントを使います。ライセンス表記やツールへの指示（`Copyright ...`、`SPDX-License-Identifier`、`eslint-disable`、`@ts-nocheck` など）は読み飛ばします。先頭のコメントが最初の宣言に直接付いている場合は、その宣言の Doc として扱います。

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

大きなリポジトリの全体像をつかむときは、`--summary` でディレクトリごとに1行で表示できます。各行の下には、そのディレクトリの説明が付きます（Go は `doc.go` を優先し、なければ package コメント、TS/JS は `index` ファイルの先頭コメント）。

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

| フラグ | 説明 |
|---|---|
| `--summary` | ディレクトリごとに1行で、ファイル数・シンボル数・主な公開名と、パッケージの説明を表示する |
| `--exported` | Goで公開シンボルだけを表示する（TS/JSは常に export された宣言のみ） |
| `--full-doc` | Docコメントを全文で表示する（ファイルやパッケージの説明は、最初の見出しの手前まで、最大15行） |
| `--no-doc` | Docコメントを表示しない（ファイルやパッケージの説明も含む） |
| `--no-lines` | `#L開始-終了` の行番号を表示しない |
| `--max-lines N` | 出力を約 N 行で打ち切る（既定は `500`、`0` で無制限）。詳しくは後述 |
| `--changed` | git で変更のあるファイル（ステージ済み・未ステージ・未追跡）だけを対象にする |
| `--base <ref>` | `--changed` と併用。`ref`（例: `main`）との分岐点以降の変更も含める。PRに含まれる変更と同じ範囲になる |

- Goの `const` / `var` は、公開されている名前だけを1行にまとめて表示します（例: `const ModeA, ModeB`）。
- TS/JSの class は、公開メンバーのシグネチャだけを表示します（`private` と `#private` は除外）。

### `ctxmap deps [path]` — ローカル依存関係

外部パッケージは除き、リポジトリ内の import だけを一覧にします。Goはパッケージディレクトリ単位、TS/JSは解決後のファイル単位で表示します。

```text
$ ctxmap deps
# deps . (local only; 4/14 files have local deps)
internal/cmd/common.go -> internal/filter, internal/parser
internal/cmd/map.go -> internal/parser
main.go -> internal/cmd
```

`--target` を付けると、指定した対象の **依存先** と **被参照元** に絞り込みます。対象はファイルパス、ディレクトリ、Goのimportパス、パッケージ名、拡張子なしのファイル名のいずれでも指定できます。
`--depth N` を付けると、依存先・被参照元の両方向に N 段先までたどります（`0` は無制限）。各行の先頭には、対象から何段離れているかが付きます。

```text
$ ctxmap deps --target filter --depth 0
# deps --target filter --depth 0
target: internal/filter/ignore.go
depends on (0):
referenced by (2):
- [1] internal/cmd/common.go
- [2] main.go
```

| フラグ | 説明 |
|---|---|
| `--target <x>` | 1つのファイル・ディレクトリ・パッケージに絞り込む |
| `--depth N` | `--target` と併用。たどる段数（既定は `1`、`0` は無制限） |
| `--changed`, `--base <ref>` | `--target` なしのときに、変更のあるファイルだけを一覧にする（意味は `map` と同じ） |
| `--max-lines N` | `--target` なしのときに、一覧を約 N 行で打ち切る（既定は `500`、`0` で無制限） |

TS/JS の import は次のように解決します。

- 相対パス: `./foo`、`./foo.js`（中身が `foo.ts` の場合）、`./dir`（`dir/index.ts`）
- いちばん近い `tsconfig.json` / `jsconfig.json` のエイリアス: `compilerOptions.paths`（例: `@/*`）と `baseUrl`。相対パスの `extends` で読み込まれる設定も対象です。設定ファイル内のコメントや末尾のカンマがあっても読めます。
- ファイルに解決できないものは、外部パッケージとして扱います。

### `ctxmap slice <symbol>... [path]` — 定義の本体

関数・メソッド・型・class・メンバーの定義を、本体とDocコメントを含めてそのまま出力します。ファイルパスと行範囲も付きます。
スペース区切りかカンマ区切りで、複数のシンボルを一度に指定できます。

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

- シンボルは `Name`、`Recv.Method`（Go）、`Class.member`（TS/JS）の形式で指定します。
- 完全一致がなければ、大文字小文字を区別せずに検索します。見つからなかったシンボルは、類似候補とあわせて stderr に出します。見つかった分は出力したうえで、1つでも見つからなければ終了コード1を返します。
- 最後の引数が実在するパスなら、それを検索対象のパスとして扱います（`ctxmap slice Map cmd`）。シンボル名がパスと紛らわしい場合は、`--path` でパスを指定してください。

### 共通事項

- フラグは位置引数の前後どちらにも書けます（`ctxmap map src --exported`）。
- 出力パスは、カレントディレクトリ配下ならそこからの相対パスで出します。そのまま Read に渡せます。
- 解析できないファイルは警告を stderr に出してスキップし、処理は続けます。
- **出力の上限**: 大きなリポジトリでAIのコンテキストがあふれないよう、`map`、`map --summary`、`deps` の一覧は、既定で約500行で出力を打ち切ります。打ち切りはファイル単位（`--summary` ではディレクトリ単位）で行うので、途中で切れた中途半端な出力にはなりません。ヘッダーには全体の件数がそのまま表示され、最後の行に、省略した量と絞り込み方の案内が出ます。

  ```text
  ... (truncated at --max-lines 500: 4808 more files, 153844 lines not shown; narrow with 'ctxmap map --summary' or a subdirectory path, or pass --max-lines 0)
  ```

  上限は `--max-lines N` で変えられ、`--max-lines 0` で無制限になります。`slice` は指定したシンボルだけを出すので、上限はありません。
- 終了コード: `0` 成功 / `1` 実行時エラー・未検出 / `2` 引数エラー
- `ctxmap version` でバージョンを表示します。

## 対象ファイルの選び方

すべてのコマンドで、次のフラグが使えます。

| フラグ | 説明 |
|---|---|
| `--include-tests` | テストファイル（`*_test.go` `*.spec.*` `*.test.*`）も対象にする |
| `--exclude <pattern>` | `.gitignore` と同じ書式の除外パターンを追加する。パスは走査対象からの相対。複数指定でき、`!pattern` で除外を打ち消せる |
| `--no-gitignore` | `.gitignore` を適用しない（`.ctxmapignore` は適用する） |

デフォルトで除外するもの:

| 種類 | パターン |
|---|---|
| ディレクトリ | `.git` `node_modules` `vendor` `dist` `build` `.next` `.claude` |
| ファイル | `go.sum` `package-lock.json` `yarn.lock` `pnpm-lock.yaml` `*.min.js` `*.pb.go` |
| テスト | `*_test.go` `*.spec.*` `*.test.*`（`--include-tests` で対象に含める） |

### `.ctxmapignore`

任意のディレクトリに `.ctxmapignore`（書式は `.gitignore` と同じ）を置くと、git の管理対象ではあってもAIに見せる必要のないものを隠せます。生成コード、`testdata/`、`examples/`、モックなどが当てはまります。コミットしておけば、チームでもCIでも同じ設定が効きます。

```gitignore
# .ctxmapignore
mocks/
testdata/
*.gen.go
# デフォルトで除外されるディレクトリを対象に戻す
!build/
```

`.gitignore` と同じく、コメントは独立した行に書いてください。パターンの後ろに書いた `# ...` は、パターンの一部として扱われます。

ルールは次の順に適用し、後のものほど優先します。

1. デフォルトの除外
2. `.gitignore`
3. `.ctxmapignore`
4. `--exclude`

`.gitignore` は除外を追加するだけです。`.gitignore` の `!` では、`vendor/` などのデフォルトの除外は打ち消せません。`.ctxmapignore` と `--exclude` では打ち消せます。
除外ファイルは、走査対象の各ディレクトリと、リポジトリルートまでの親ディレクトリから読み込みます。
`*`、`**`、`?`、`[...]`、否定の `!`、末尾 `/`（ディレクトリのみ）、先頭 `/`（アンカー）に対応した簡易実装です。

## AIエージェントでの使い方

### Claude Code のスキル（おすすめ）

[skills/ctxmap/SKILL.md](skills/ctxmap/SKILL.md) は、[Claude Code のスキル](https://docs.claude.com/ja/docs/claude-code/skills)です。下の基本の流れ、出力の読み方（`#L` の範囲、`> ` の説明、打ち切りの案内）、コマンドの使い方をAIに教えます。ユーザースキルとして一度入れれば、すべてのプロジェクトで使えます。読み取り専用の `ctxmap` コマンドは事前に許可されるので、実行のたびに確認は出ません。

```sh
# このリポジトリを clone している場合
make install-skill
# clone せずに入れる場合
mkdir -p ~/.claude/skills/ctxmap
curl -fsSL https://raw.githubusercontent.com/withnic/ctxmap/main/skills/ctxmap/SKILL.md -o ~/.claude/skills/ctxmap/SKILL.md
```

入れた後は、Claude Code のセッションを新しく始めてください。スキルは、AIが必要だと判断したときに読み込まれます。必ず使わせたいプロジェクトでは、そのプロジェクトの `CLAUDE.md` に次の行を足してください。

```markdown
## コード探索
コードを探索するときは、ファイルを Read する前に ctxmap スキルを使うこと（構成の把握・定義の検索・影響範囲の確認・変更のレビュー）。
```

### スキルを使わない場合

[doc/ctxmap_rules_template.md](doc/ctxmap_rules_template.md) に、同じ内容のルールを CLAUDE.md に貼れる形で用意しています。

### 基本の流れ

1. `ctxmap map` で全体を把握する（大きなリポジトリでは先に `ctxmap map --summary`）
2. `ctxmap deps --target` で影響範囲を確かめる（間接的な影響は `--depth` で）
3. `ctxmap slice` で必要な定義だけを読む（複数まとめて指定できる）。または `map` の `#L` の範囲だけを Read する
4. ファイル全体の Read は最後の手段にする

レビューでは、`ctxmap map --changed --base main` でブランチが変更したファイルの骨格だけを確認できます。

## 開発

GNU make と POSIX シェルが必要です（Windowsでは Git Bash から実行してください）。golangci-lint と govulncheck は Makefile でバージョンを固定しており、初回実行時に `./bin/tools` へ自動でインストールされます。

```sh
make help     # ターゲット一覧
make check    # fmt-check + vet + lint + test（コミット前に実行）
make ci       # tidy-check も含む全チェック + ビルド + スモークテスト
make cover    # カバレッジ
make vuln     # 既知の脆弱性スキャン
make install-skill  # Claude Code のスキルを ~/.claude/skills にコピー
make build VERSION=v0.1.0
```

Lintの設定は [.golangci.yml](.golangci.yml) にあります。standard セットに加えて、revive、gocritic、errorlint、misspell、unconvert、unparam、nolintlint を有効にしています。フォーマッタは gofmt と goimports です。

### ディレクトリ構成

```text
main.go          エントリーポイントとサブコマンドの振り分け
internal/cmd/    map / deps / slice、共通フラグ、git の差分取得、tsconfig の解決
internal/parser/ Go（go/ast）と TS/JS（軽量パーサー）の解析
internal/filter/ デフォルトの除外、.gitignore / .ctxmapignore / --exclude の判定
doc/             要件・設計・ガイドライン・CLAUDE.md 用テンプレート
skills/ctxmap/   Claude Code のスキル（`make install-skill`）
```

設計の詳細は [doc/01_system_overview.md](doc/01_system_overview.md)、[doc/02_architecture.md](doc/02_architecture.md)、[doc/03_guidelines.md](doc/03_guidelines.md) を参照してください。

## 既知の制約

- TS/JSは厳密なAST解析ではありません。JSXのテキストに `'` などが含まれると、その行の解析がまれに崩れることがあります。
- tsconfig の `extends` は相対パスのものだけをたどります。パッケージの設定（例: `@tsconfig/node20`）は読みません。
- `--changed` を使うには、`PATH` に `git` があり、コミットが1つ以上あるリポジトリである必要があります。

## バージョンの方針

ctxmap は [セマンティック バージョニング](https://semver.org/lang/ja/) に従います。互換性の対象は CLI です。具体的には、コマンド名、フラグ、終了コード、AIが読む出力形式が対象です（パッケージはすべて `internal/` 配下にあり、import できる Go の API はありません）。

- **v1.0.0 より前**は、マイナーリリース（`v0.x.0`）でフラグや出力形式が変わることがあります。変更内容は [CHANGELOG.md](CHANGELOG.md) に記載します。パッチリリース（`v0.x.y`）は不具合の修正だけです。
- **v1.0.0 以降**は、互換性を壊す変更はメジャーバージョンを上げて行います。

再現性が必要な場合は、`go install github.com/withnic/ctxmap@v0.1.0` のようにバージョンを固定してください。

## ライセンス

[MIT](LICENSE)
