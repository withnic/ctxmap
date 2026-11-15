# ctxmap - Claude Code エージェント用指示書

## プロジェクト概要
AIがコードベースを探索する際、ファイル全体を何度も Read してトークンを浪費するのを防ぐCLIです。コードの「骨格（シグネチャ・型定義・Docコメント要約）」と「依存関係」だけを、トークン効率のよいMarkdownで標準出力に返します。

- モジュール: `github.com/withnic/ctxmap`（公開リポジトリ: https://github.com/withnic/ctxmap ）
- サブコマンド: `map` / `deps` / `slice` / `version`（使い方は [README.ja.md](README.ja.md)）
- 対応言語: Go のみ（`go/ast`）。TypeScript/JavaScript への対応は v0.2.0 で終了した（推測による解析の取りこぼしが、AIにとって危険なため）
- 要件・設計・ガイドライン: [doc/01_system_overview.md](doc/01_system_overview.md)、[doc/02_architecture.md](doc/02_architecture.md)、[doc/03_guidelines.md](doc/03_guidelines.md)

## 構成
```text
main.go              エントリーポイント（サブコマンドの振り分け、version）
internal/cmd/        map / deps / slice、共通フラグ、git 差分
internal/parser/     Go のパーサー（go/ast）
internal/filter/     デフォルト除外、.gitignore / .ctxmapignore / --exclude
doc/                 要件・設計・ガイドライン・他プロジェクト用 CLAUDE.md テンプレート
skills/ctxmap/       Claude Code のスキル（`make install-skill` で ~/.claude/skills に入る）
```
パッケージはすべて `internal/` 配下に置き、公開APIにはしない（提供するのはCLIだけ）。

## 開発ルール
- **外部依存を追加しない**。標準ライブラリだけで実装し、CGO なしで `go install` できる状態を保つ。
- **最低サポートは Go 1.22**（`go.mod` の `go` ディレクティブ）。それより新しい言語機能・標準ライブラリAPIは使わない。CI が Go 1.22 で ubuntu / macOS / Windows のテストを実行する。
- 除外ルールは [doc/03_guidelines.md](doc/03_guidelines.md) を守る。解析できないファイルは stderr に警告を出してスキップし、処理は止めない。
- 出力はトークン効率を最優先する（JSON は使わない）。出力形式を変えたら、README の出力例も更新する。
- パスの比較は OS の違いに注意する（macOS のシンボリックリンク、Windows の短縮名・大文字小文字）。
- フラグや挙動を変えたら、次をあわせて更新する。
  - [README.md](README.md)（英語）と [README.ja.md](README.ja.md)（日本語）。両方の章立てを揃える。
  - `main.go` の usage
  - [skills/ctxmap/SKILL.md](skills/ctxmap/SKILL.md)（Claude Code のスキル。こちらが正）と [doc/ctxmap_rules_template.md](doc/ctxmap_rules_template.md)（スキルを使わない人向けの同じ内容のルール）
- 公開リポジトリなので、コミット・PR・README・CHANGELOG に、ローカルにある別プロジェクトの名前やパス、PC固有の情報を書かない。計測の例には Go 標準ライブラリなど公開のコードを使う。
- テストは各パッケージの `*_test.go` に書く。git を使うテストは、git がない環境ではスキップする。

## コマンド
```sh
make help     # ターゲット一覧
make check    # fmt-check + vet + lint + test（コミット前に必ず通す）
make ci       # CI と同じ全チェック + ビルド + スモークテスト
make fmt      # 整形（gofmt + goimports）
make cover    # カバレッジ
```
Windows では Git Bash から実行する。golangci-lint と govulncheck は Makefile でバージョンを固定しており、初回に `./bin/tools` へ自動で入る。

## コード探索ルール（このリポジトリでも ctxmap を使う）

コードを読む前に ctxmap で骨格と依存関係を確認すること。ファイル全体の Read は最後の手段とする。
このリポジトリは ctxmap 自身なので、インストール済みの `ctxmap` ではなく **`go run .`** を使い、作業ツリーの最新の実装で確認する（以下の `go run . <command>` は、インストール済みなら `ctxmap <command>` と同じ）。

### 基本フロー
1. **全体把握**: 最初に `go run . map` を1回だけ実行し、型定義・関数シグネチャ・Docコメント要約を把握する。パッケージ単位で見るなら `go run . map internal/cmd` のように絞る。
   - ファイル見出しの直下の `> ` で始まる行はそのファイルの役割、`--summary` の各ディレクトリの下の `> ` 行はパッケージの役割。これを見て、読むファイルを選ぶ。
2. **影響範囲の確認**: 変更対象を決めたら `go run . deps --target <file|dir|package>` で依存先と被参照元を確認する。間接的な影響は `--depth 0`（無制限）で確認する。
3. **実装の確認**: 中身が必要な関数・型だけを `go run . slice <Symbol> [<Symbol>...]` で取得する。複数のシンボルは1回でまとめて取得する（例: `go run . slice Walk compileRule`）。
4. **Read は限定的に**: `map` の各宣言にある `#L開始-終了` を使い、Read は `offset`/`limit` をその範囲に絞る。ファイル全体を Read するのは、編集直前のファイルと、Go 以外のファイル（Makefile、YAML、Markdown など）だけにする。

### 作業中・レビュー時
- 未コミットの変更だけを見る: `go run . map --changed`
- ブランチ全体の変更を見る: `go run . map --changed --base main`
- テストコードも見る: 各コマンドに `--include-tests` を付ける

### 守ること
- 同じセッションで `map` を何度も実行しない。編集した後は `--changed` で変更分だけを再確認する。
- 出力の末尾に `... (truncated at --max-lines ...)` が出たら、`--max-lines 0` で全部を取得するのではなく、`--summary` やサブディレクトリの指定で対象を絞る。
- シンボル名が不確かな場合は先に `map` で確認する。`slice` で見つからなかったシンボルは、出力の末尾に `> [not found]` として類似の候補とともに出るので、それを使う。1語の名前のディレクトリで絞り込むときは `--path <dir>` を使う（最後の引数がパスとみなされるのは `./dir` のように明らかにパスの場合だけ）。型を `slice` すると、その型のメソッド一覧も付くので、メソッドを探すために別途 `map` しなくてよい。
