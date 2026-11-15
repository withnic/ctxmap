---
name: ctxmap
description: Explore a Go codebase with the ctxmap CLI instead of reading whole files. Use BEFORE opening .go files whenever you need to understand a Go project's structure, find where a function, method or type is defined, see what a package is for, check what depends on something (impact of a change), or review changed files on a branch. Prefer it over Read/Grep for code exploration in Go projects (go.mod present). Not for other languages.
allowed-tools: Bash(ctxmap *), Bash(go run . map *), Bash(go run . deps *), Bash(go run . slice *), PowerShell(ctxmap *), PowerShell(go run . map *), PowerShell(go run . deps *), PowerShell(go run . slice *)
---

# ctxmap によるコード探索

ctxmap は、Go のコードの骨格（シグネチャ・型定義・Docコメントの要約）と依存関係だけを、トークン効率のよい Markdown で返す CLI です。ファイル全体を Read する前に、これで必要な場所を絞り込みます。

## 0. 使えるか確認する
- `ctxmap version` を一度だけ実行する。
- 見つからない場合は、ユーザーに `go install github.com/withnic/ctxmap@latest` を案内し、それまでは通常の Read / Grep で進める（勝手にインストールしない）。
- **ctxmap 自身のリポジトリ**（`go.mod` が `module github.com/withnic/ctxmap`）では、インストール済みの版ではなく `go run . <command>` を使う（作業ツリーの最新の実装を使うため）。

## 1. 基本の流れ
1. **全体を把握する**: `ctxmap map [dir]` を1回だけ実行する。大きなリポジトリでは先に `ctxmap map --summary` でディレクトリ構成をつかみ、対象のディレクトリに絞って `map` する（例: `ctxmap map src/api`）。
2. **影響範囲を確かめる**: 変更対象が決まったら `ctxmap deps --target <file|dir|package>` で、依存先と被参照元を確認する。間接的な影響は `--depth 2`（`0` で無制限）。依存関係を知るために import 文を Read / Grep で追わない。
3. **実装を読む**: 中身が必要な関数・型だけを `ctxmap slice <Symbol> [<Symbol>...]` で取得する。複数のシンボルは1回でまとめて取得する。メソッドは `Recv.Method` で指定する。
4. **Read は最後に、範囲を絞って**: `map` の各宣言にある `#L開始-終了` を使い、Read の `offset` / `limit` をその範囲に絞る。ファイル全体を Read するのは、編集する直前のファイルと、ctxmap が対応していないファイル（設定ファイル、Markdown など）だけにする。

## 2. 出力の読み方
| 出力 | 意味 |
|---|---|
| `## path/to/file.go [pkg]` | ファイルの見出し（パッケージ名付き）。`(go:build linux)` などが付いていれば、そのプラットフォーム専用のファイル |
| 見出し直下の `> ...` | そのファイルの役割（package コメント） |
| `--summary` の各行の下の `  > ...` | そのディレクトリ（パッケージ）の役割。読むディレクトリを選ぶのに使う |
| `// ...` | 次の宣言の Doc コメント（既定は最初の1文） |
| 宣言末尾の `#L14-86` | その宣言の行範囲（Doc コメントを含む）。Read の範囲指定にそのまま使う |
| 末尾の `... (truncated at --max-lines 500: ...)` | 出力が上限（既定500行）で打ち切られた。`--max-lines 0` で全部を取るのではなく、`--summary` やサブディレクトリの指定で対象を絞る |

## 3. 作業中・レビュー
- 未コミットの変更だけを見る: `ctxmap map --changed`
- ブランチ（PR）全体の変更を見る: `ctxmap map --changed --base main`
- 変更ファイルの依存関係: `ctxmap deps --changed --base main`

## 4. コマンド早見表
| 目的 | コマンド |
|---|---|
| ディレクトリごとの概要 | `ctxmap map --summary` |
| 骨格（シグネチャ・型・Doc要約・行番号） | `ctxmap map [path]` |
| 公開APIだけ | `ctxmap map [path] --exported` |
| Docコメント全文 / なし | `--full-doc` / `--no-doc` |
| ローカルの依存グラフ | `ctxmap deps [path]` |
| 特定の対象の依存先・被参照元 | `ctxmap deps --target <file|dir|package> [--depth N]` |
| 定義の本体を取得（複数可） | `ctxmap slice <Name|Recv.Method>... [path]` |
| テストも対象にする | `--include-tests`（全コマンド共通） |
| その場で除外する | `--exclude '<pattern>'`（全コマンド共通、複数指定可） |
| 出力の上限を変える | `--max-lines N`（`map` と `deps`、既定500、`0` で無制限） |
| 行番号を出さない / .gitignore を無視する | `--no-lines` / `--no-gitignore` |
| シンボル名がパスと紛らわしいとき | `ctxmap slice <Symbol> --path <dir>` |

## 5. 守ること
- 同じセッションで同じディレクトリの `map` を繰り返さない（前回の出力を使う）。編集した後は `--changed` で変更分だけを確認する。
- シンボル名が不確かなら、先に `map` で確認する。`slice` で見つからなかったシンボルは、出力の末尾に `> [not found]` として類似の候補とともに出るので、それを使う。1語の名前のディレクトリで絞り込むときは `--path <dir>` を使う（最後の引数がパスとみなされるのは `./dir` のように明らかにパスの場合だけ）。型を `slice` すると、その型のメソッド一覧も付くので、メソッドを探すために別途 `map` しなくてよい。
- 対応言語は **Go のみ**。それ以外の言語のプロジェクトでは、このスキルは使わずに通常どおり探索する。TypeScript では、必要なら `npx tsc --declaration --emitDeclarationOnly --outDir <一時ディレクトリ>` で型の骨格（`.d.ts`）を出せる（プロジェクトのファイルは変更しない）。
- `.git` `vendor` `testdata` `node_modules` `.claude`、`.gitignore` と `.ctxmapignore` の対象は自動で除外される。生成コードなど、見る必要のないものが出てくる場合は、プロジェクトの `.ctxmapignore` への追記をユーザーに提案する。
